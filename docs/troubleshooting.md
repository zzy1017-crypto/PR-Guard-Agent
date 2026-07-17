# PR-Guard-Agent 排错手册

以下问题来自本项目的 PowerShell、Redis、RAG、缓存与异步任务链路。命令中的 ID、路径、容器名应替换为当前环境的真实值。

## PowerShell `curl` JSON 转义失败

**现象**：服务返回 `invalid json body`，或 PowerShell 把 `-H`、`-d` 当作自己的参数解析。

**根因**：Windows PowerShell 中 `curl` 可能是 `Invoke-WebRequest` 的别名；多层引号和反斜杠使 JSON 在到达服务前已被改写。

**检查命令**：

```powershell
Get-Command curl
$body = @{ text = "hello" } | ConvertTo-Json
$body
```

**修复方式**：使用对象序列化，不手写 `curl -d`：

```powershell
$body = @{ text = "hello" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/embedding/test" -ContentType "application/json" -Body $body
```

**工程教训**：脚本应把请求对象和序列化交给 PowerShell，避免依赖 Shell 特定转义规则。

## HTTP 4xx 被 `Invoke-RestMethod` 显示为红色异常

**现象**：预期的 400/404/429 被显示为红色异常，看起来像脚本崩溃。

**根因**：`Invoke-RestMethod` 对非 2xx 状态抛出终止或非终止错误，而不是正常返回响应对象。

**检查命令**：

```powershell
try {
    Invoke-RestMethod "http://localhost:8080/analysis-tasks/0"
} catch {
    $_.Exception.Response.StatusCode.value__
    $_.ErrorDetails.Message
}
```

**修复方式**：故障断言用 `try/catch` 读取状态码和 `ErrorDetails.Message`；不要仅凭红色文本判断系统异常。

**工程教训**：测试必须明确区分“HTTP 负向用例通过”和“请求工具执行失败”。

## `System.Object[]` 展开问题

**现象**：输出出现 `System.Object[]`，无法看到 `items`、`workers` 或 Redis Key 的真实内容。

**根因**：数组被放入字符串插值时调用了默认 `ToString()`，嵌套对象也不会自动展开。

**检查命令**：

```powershell
$response = Invoke-RestMethod "http://localhost:8080/ops/workers"
$response.data.workers | Format-Table -AutoSize
$response.data.workers | ConvertTo-Json -Depth 8
```

**修复方式**：通过管道格式化/序列化；字符串中需要列表时使用 `($values -join ', ')`。

**工程教训**：终端展示和断言应针对对象字段，不应依赖隐式字符串转换。

## Git diff 被保存为 UTF-16

**现象**：明明包含 `diff --git`，上传仍报 diff header 或 hunk 解析失败；文件开头可见 NUL 字节。

**根因**：Windows PowerShell 5.1 的部分输出方式默认写 UTF-16LE，解析器期望 UTF-8 文本。

**检查命令**：

```powershell
Format-Hex -Path .\change.diff | Select-Object -First 2
Get-Content -Raw .\change.diff | Select-String "diff --git"
```

**修复方式**：直接让 Git 输出 UTF-8 文件，或显式转换：

```powershell
git diff | Out-File -FilePath .\change.diff -Encoding utf8
$text = Get-Content -Raw .\change.diff
[System.IO.File]::WriteAllText((Resolve-Path .\change.diff), $text, [System.Text.UTF8Encoding]::new($false))
```

**工程教训**：上传文本文件时要把编码作为测试前置条件，而不只检查文件扩展名。

## `diff_id` 不属于 `project_id`

**现象**：Retrieve、Analyze 或异步提交返回 `diff does not belong to project`。

**根因**：路径中的项目 ID 与 `diff_records.project_id` 不一致，常见于复用旧终端变量。

**检查命令**：

```powershell
docker exec prguard-mysql mysql -uroot -p pr_guard -e "SELECT id, project_id, diff_hash FROM diff_records WHERE id=<DIFF_ID>;"
```

命令会交互式要求密码，避免把密码写进历史记录。

**修复方式**：使用上传 diff 返回的 `data.project_id` 和 `data.diff_id` 作为一组参数。

**工程教训**：跨资源 API 必须在服务端校验归属，客户端也应把关联 ID 成组保存。

## Mock Embedding 召回 `go.sum` 等低价值 Chunk

**现象**：Retrieve 的高分结果是依赖清单或配置，而不是预期业务符号。

**根因**：Mock 向量由文本哈希确定，只用于链路可重复性，不具有真实语义相似度。

**检查命令**：

```powershell
$r = Invoke-RestMethod -Method Post "http://localhost:8080/projects/<PROJECT_ID>/diffs/<DIFF_ID>/retrieve?top_k=10"
$r.data.context_chunks | Select-Object score,file_path,symbol_name
```

**修复方式**：联调阶段接受这一限制；真实评估时配置合适的 Embedding Provider，重新 Index，并记录模型/维度。不要把 Mock 结果描述为真实召回效果。

**工程教训**：确定性 Mock 验证控制流，不验证模型质量。

## `redis-cli` 与 Go 服务连接到不同 Redis

**现象**：`redis-cli` 查不到 Key，但 Analyze 返回 cached=true；或 TTL 与配置不符。

**根因**：Go 配置可能连接宿主机映射端口，而 CLI 连到另一个本机/容器实例或不同 DB。

**检查命令**：

```powershell
Select-String -Path .\configs\config.yaml -Pattern "addr:|db:"
docker port prguard-redis
docker exec prguard-redis redis-cli -n 0 INFO server
docker exec prguard-redis redis-cli -n 0 --scan --pattern "prguard:report:*"
```

**修复方式**：统一容器、端口和 DB；DAY21 缓存脚本固定使用 `docker exec prguard-redis redis-cli -n 0`。

**工程教训**：缓存证据必须同时记录连接目标、DB、Key 和 TTL。

## 旧缓存导致第一次请求 `cached=true`

**现象**：端到端脚本第一轮 Analyze 直接失败，响应中 `cached=true`。

**根因**：相同 `project_id + code_version_hash + diff_hash + top_k` 的 Key 在此前运行中仍有效。

**检查命令**：

```powershell
docker exec prguard-redis redis-cli -n 0 --scan --pattern "prguard:report:<PROJECT_ID>:*:topk:<TOP_K>"
```

**修复方式**：运行 `scripts/day21-cache-test.ps1` 删除限定项目和测试 TopK 的 Key。不要把 `FLUSHDB` 作为默认清理方式。

**工程教训**：可重复测试必须显式管理前置状态，同时把清理范围限制到测试数据。

## Redis JSON 内 `cached=false` 是正常设计

**现象**：读取 Redis 值看到 `"cached":false`，误以为缓存无效。

**根因**：缓存保存的是首次生成结果；命中时服务在响应对象上改为 `cached=true`，不是把存储值重写为 true。

**检查命令**：

```powershell
$key = docker exec prguard-redis redis-cli -n 0 --scan --pattern "prguard:report:<PROJECT_ID>:*:topk:<TOP_K>" | Select-Object -First 1
docker exec prguard-redis redis-cli -n 0 GET $key
```

**修复方式**：以本次 HTTP 响应的 `data.cached` 判断命中；Redis JSON 的 false 不需要修复。

**工程教训**：业务结果字段与本次请求元数据应区分理解。

## 不同 `top_k` 曾共享报告缓存

**现象**：先请求 top_k=5，再请求 top_k=8，第二次意外返回 cached=true 且上下文没有变化。

**根因**：旧实现若未把 `top_k` 放入 Key，会把不同模型输入视为同一分析。

**检查命令**：

```powershell
docker exec prguard-redis redis-cli -n 0 --scan --pattern "prguard:report:<PROJECT_ID>:*"
go test ./pkg/cache -run TestBuildReportCacheKey -v
go test ./internal/service -run TopK -v
```

**修复方式**：当前 Key 末尾必须是 `:topk:<N>`；升级后删除旧格式的测试缓存并重新生成。

**工程教训**：所有会改变输出的输入都必须参与缓存身份设计和回归测试。

## Redis Key 存在不能证明某次请求命中

**现象**：测试只截图了 Key，却无法说明某个 HTTP 请求是否从缓存返回。

**根因**：Key 可能由更早的请求创建；当前请求也可能因反序列化错误、降级保护或连接问题走了重新分析。

**检查命令**：

```powershell
$response = Invoke-RestMethod -Method Post "http://localhost:8080/projects/<PROJECT_ID>/diffs/<DIFF_ID>/analyze?top_k=<TOP_K>"
$response.data | Select-Object cached,degraded,report_id
```

**修复方式**：同时保存请求时间、响应 `cached`、Request ID、对应 `report_cache_hit/miss` 日志、Key 和 TTL。

**工程教训**：状态存在性证据和单次请求路径证据不是同一件事。

## 异步任务状态表的正常判断方式

**现象**：看到 pending 就认为 Worker 未工作，或看到 `attempt_count>0` 就认为任务失败。

**根因**：pending 既可能是首次排队，也可能是带 `next_run_at` 的重试；running 是已领取；Fallback 会以 succeeded + degraded=true 完成。

**检查命令**：

```powershell
$task = Invoke-RestMethod "http://localhost:8080/analysis-tasks/<TASK_ID>"
$task.data | Format-List *
Invoke-RestMethod "http://localhost:8080/ops/workers" | ConvertTo-Json -Depth 8
Invoke-RestMethod "http://localhost:8080/ops/analysis-tasks/metrics" | ConvertTo-Json -Depth 8
```

**修复方式**：按以下组合判断：首次 pending 通常 `attempt_count=0`；重试 pending 为 `retry_scheduled=true + next_run_at + last_error_code`；running 应有 `started_at`；succeeded 查看 `degraded/report_id/result`；failed 查看 `last_error_code` 和是否 `retry_exhausted`。

**工程教训**：异步系统要以状态、尝试次数、调度时间和错误码的组合判断，不以单字段下结论。
