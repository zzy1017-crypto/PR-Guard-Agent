# PR-Guard-Agent

PR-Guard-Agent 是一个面向 Go 项目的 PR 变更风险评估系统。第一周目标是先打通基础链路：项目上传、代码文件入库、Go AST 语义分块、Git diff 入库与结构化解析，为后续风险分析能力准备数据底座。

> 当前 README 是 Day7 第一周成果整理初版。文档中的 `<PROJECT_ID>`、`<FILE_COUNT>`、`<CHUNK_COUNT>`、`<DIFF_HASH>` 等均为占位符，请根据本地真实运行结果替换。

## 技术栈

- Go 1.25.4
- Gin：HTTP API
- GORM：MySQL ORM 与 AutoMigrate
- MySQL 8.0：项目、文件、代码块、diff、风险报告元数据
- Redis 7.2：基础连接初始化，后续可用于缓存
- Qdrant：Docker Compose 已预留服务，当前业务链路尚未使用
- Viper：YAML 配置加载
- Docker Compose：本地 MySQL、Redis、Qdrant 编排
- Go AST：对 `.go` 文件进行函数、方法、结构体、接口、常量、变量级别的语义分块
- SHA-256：计算文件内容 hash、项目版本 hash、diff hash

## 当前已完成功能

- `GET /health` 健康检查
- `docker-compose.yml` 启动 MySQL、Redis、Qdrant
- 启动服务时初始化 MySQL、Redis
- GORM AutoMigrate 以下模型：
  - `Project`
  - `ProjectFile`
  - `CodeChunk`
  - `DiffRecord`
  - `RiskReport`
- `POST /projects/upload`
  - 上传 Go 项目 zip
  - 过滤允许的源码/配置文件
  - 解压到 `data/projects/<PROJECT_ID>`
  - 保存 `projects`、`project_files`
  - 计算每个文件的 `content_hash`
  - 计算项目级 `code_version_hash`
- `POST /projects/:id/chunks/ast`
  - 读取项目下已入库的 `.go` 文件
  - 基于 Go AST 生成语义代码块
  - 保存到 `code_chunks`
- `POST /projects/:id/diffs`
  - 上传 `.diff`、`.patch` 文件或 `diff_text`
  - 计算 `diff_hash`
  - 保存到 `diff_records`
  - 解析 `changed_files`、`hunks`、变更行类型和行号

当前尚未实现：Embedding、Qdrant 向量写入或检索、RAG、LLM 风险分析、`analyze` 接口。

## 第一周核心流程

1. 启动基础设施：MySQL、Redis、Qdrant。
2. 启动 Gin 后端服务。
3. 服务读取 `configs/config.yaml`，初始化 MySQL 和 Redis。
4. 服务执行 AutoMigrate，确保基础表结构存在。
5. 上传 Go 项目 zip，系统过滤文件并持久化项目信息。
6. 生成 Go AST 语义分块，写入 `code_chunks`。
7. 上传 Git diff，系统保存 diff 原文并解析变更文件和 hunk。

当前文件过滤按代码实现保留以下扩展名：`.go`、`.md`、`.yaml`、`.yml`、`.josn`、`.lua`、`.sql`、`.mod`、`.sum`；忽略目录包括 `.git`、`vendor`、`node_modules`、`tmp`、`dist`、`.idea`、`.vscode`。

## 接口说明

### GET /health

检查服务是否启动。

```powershell
curl.exe http://localhost:8080/health
```

响应示例：

```json
{
  "code": 0,
  "msg": "pr-guard-agent is running"
}
```

### POST /projects/upload

上传 Go 项目 zip。请求类型为 `multipart/form-data`。

请求参数：

- `project_name`：项目名称，必填
- `file`：zip 文件，必填，仅支持 `.zip`，最大 20MB

```powershell
curl.exe -X POST http://localhost:8080/projects/upload `
  -F "project_name=<PROJECT_NAME>" `
  -F "file=@<GO_PROJECT_ZIP_PATH>"
```

响应示例：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "project_id": "<PROJECT_ID>",
    "project_name": "<PROJECT_NAME>",
    "file_count": "<FILE_COUNT>"
  }
}
```

### POST /projects/:id/chunks/ast

对指定项目已入库的 `.go` 文件执行 AST 语义分块。

```powershell
curl.exe -X POST http://localhost:8080/projects/<PROJECT_ID>/chunks/ast
```

响应示例：

```json
{
  "code": 0,
  "msg": "ast chunks generated",
  "data": {
    "project_id": "<PROJECT_ID>",
    "chunk_count": "<CHUNK_COUNT>"
  }
}
```

### POST /projects/:id/diffs

上传 Git diff。请求可以使用 `.diff`/`.patch` 文件，也可以使用表单字段 `diff_text`。文件最大 50MB。

使用文件上传：

```powershell
curl.exe -X POST http://localhost:8080/projects/<PROJECT_ID>/diffs `
  -F "file=@<DIFF_OR_PATCH_PATH>"
```

使用文本上传：

```powershell
curl.exe -X POST http://localhost:8080/projects/<PROJECT_ID>/diffs `
  -F "diff_text=<GIT_DIFF_TEXT>"
```

响应示例：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "diff_id": "<DIFF_ID>",
    "project_id": "<PROJECT_ID>",
    "diff_hash": "<DIFF_HASH>",
    "changed_files": [
      {
        "file_path": "<CHANGED_FILE_PATH>",
        "old_path": "<OLD_FILE_PATH>",
        "new_path": "<NEW_FILE_PATH>",
        "changed_type": "<added|deleted|modified>",
        "hunks": [
          {
            "old_start": "<OLD_START>",
            "old_count": "<OLD_COUNT>",
            "new_start": "<NEW_START>",
            "new_count": "<NEW_COUNT>",
            "lines": []
          }
        ]
      }
    ]
  }
}
```

错误响应统一形态：

```json
{
  "code": 1,
  "msg": "<ERROR_MESSAGE>"
}
```

## 启动方式

启动依赖服务：

```powershell
docker compose up -d
```

确认容器状态：

```powershell
docker compose ps
```

启动后端：

```powershell
go run ./cmd
```

默认配置：

- API: `http://localhost:8080`
- MySQL: `localhost:13306`
- Redis: `localhost:6379`
- Qdrant HTTP: `localhost:6333`
- Qdrant gRPC: `localhost:6334`

配置文件位置：`configs/config.yaml`。

## 测试命令

运行 Go 测试：

```powershell
go test ./...
```

只运行 diff parser 测试：

```powershell
go test ./pkg/parser -run TestParseDiff -v
```

手动 API 冒烟测试：

```powershell
curl.exe http://localhost:8080/health
curl.exe -X POST http://localhost:8080/projects/upload -F "project_name=<PROJECT_NAME>" -F "file=@<GO_PROJECT_ZIP_PATH>"
curl.exe -X POST http://localhost:8080/projects/<PROJECT_ID>/chunks/ast
curl.exe -X POST http://localhost:8080/projects/<PROJECT_ID>/diffs -F "file=@<DIFF_OR_PATCH_PATH>"
```

也可以使用脚本：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\day7-api-smoke.ps1 `
  -ProjectName "<PROJECT_NAME>" `
  -ProjectZip "<GO_PROJECT_ZIP_PATH>" `
  -ProjectId "<PROJECT_ID>" `
  -DiffFile "<DIFF_OR_PATCH_PATH>"
```

## 数据库检查命令

查看表：

```powershell
docker exec prguard-mysql mysql -uroot -p123456 pr_guard -e "SHOW TABLES;"
```

查看项目：

```powershell
docker exec prguard-mysql mysql -uroot -p123456 pr_guard -e "SELECT id, name, code_version_hash, create_at FROM projects ORDER BY id DESC LIMIT 5;"
```

查看指定项目文件数：

```powershell
docker exec prguard-mysql mysql -uroot -p123456 pr_guard -e "SELECT COUNT(*) AS file_count FROM project_files WHERE project_id=<PROJECT_ID>;"
```

查看指定项目代码块数：

```powershell
docker exec prguard-mysql mysql -uroot -p123456 pr_guard -e "SELECT COUNT(*) AS chunk_count FROM code_chunks WHERE project_id=<PROJECT_ID>;"
```

查看指定项目 diff 记录：

```powershell
docker exec prguard-mysql mysql -uroot -p123456 pr_guard -e "SELECT id, project_id, diff_hash, create_at FROM diff_records WHERE project_id=<PROJECT_ID> ORDER BY id DESC LIMIT 5;"
```

也可以使用脚本：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\db-check.ps1 -ProjectId "<PROJECT_ID>"
```

## 常见问题

### MySQL 连接失败

先确认容器已启动，且 `configs/config.yaml` 中 MySQL 端口为 `13306`：

```powershell
docker compose ps
```

### Redis 连接失败

确认 `prguard-redis` 正常运行，且配置中的地址为 `localhost:6379`。

### 上传项目返回 file is required

请确认请求使用 `multipart/form-data`，字段名为 `file`，并且文件路径可被当前 PowerShell 会话访问。

### 上传项目返回 only .zip file is allowed

当前项目上传接口只接受 `.zip` 后缀文件。

### 上传项目后 file_count 与预期不同

系统只保存白名单扩展名文件，并会跳过 `.git`、`vendor`、`node_modules` 等目录。请以当前文件过滤规则为准。

### AST chunk_count 为 0

当前 AST 分块只处理 `.go` 文件，并要求 Go 文件可以被 `go/parser` 正常解析。

### diff 上传失败

当前 diff parser 期望 Git unified diff 格式，通常需要包含 `diff --git` 文件头和 `@@ ... @@` hunk 头。

### go test ./... 扫描到 data/projects

上传后的项目会落在 `data/projects/<PROJECT_ID>`，如果其中包含 `.go` 文件，`go test ./...` 可能会把它们也作为当前模块下的包扫描。若本地上传样例会影响测试判断，可以先运行更聚焦的命令，例如 `go test ./pkg/parser -run TestParseDiff -v`。

### Qdrant 为什么没有数据

Qdrant 当前仅由 Docker Compose 启动并预留给后续阶段，第一周链路尚未写入向量数据。

## 后续计划

- 补充项目上传、AST 分块、diff 上传的接口测试。
- 明确文件过滤白名单是否需要配置化。
- 建立 diff 变更行与 AST 代码块之间的关联规则。
- 完善风险报告生成前的数据契约与表结构使用方式。
- 在基础链路稳定后，再进入向量化、检索增强和模型分析阶段。
