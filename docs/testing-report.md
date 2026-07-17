# PR-Guard-Agent DAY21 实测报告

本报告记录 2026-07-17 的实际执行结果。除明确标注外，Provider 均为 Mock；Mock 结果只证明链路行为，不代表真实语义分析质量。测试期间按场景临时修改配置并逐次重启，结束后已恢复常规开发配置。

## 测试元数据

| 字段 | 实际值 |
|---|---|
| 测试日期与时区 | 2026-07-17，Asia/Shanghai（UTC+08:00） |
| Git Commit | `eca107bf4e844a39eda3755e435c66b8ce43dfd7`；测试时工作区包含未提交 DAY21 修改 |
| 操作系统 | Microsoft Windows NT 10.0.26200.0，64 位 |
| CPU / 内存 | Intel64 Family 6 Model 170 Stepping 4；18 逻辑处理器；内存因沙箱账户无 Win32 CIM 权限未取得，需要人工补充 |
| Go | `go1.25.4 windows/amd64` |
| Docker / Compose | Docker 29.4.3（build 055a478）/ Compose v5.1.3 |
| MySQL / Redis / Qdrant | MySQL 8.0.46 / Redis 7.2.14 / Qdrant 1.18.2 |
| Provider | Embedding `mock/mock_embedding`；LLM `mock/mock-llm` |
| 恢复后的关键配置 | worker_count=2、task timeout=30s、stale=60s、max_attempts=3、LLM timeout=5s、mock delay=1000ms、rate_limit=100/60s、cache TTL=3600s |
| 主测试数据 | project_id=6、diff_id=5；新增上传 project_id=8、diff_id=6 |

## 功能、故障与可观测性记录

| 测试接口/项目 | 测试条件 | 实际结果 | 结论 | 证据位置 |
|---|---|---|---|---|
| 健康检查与 Request ID | 自动生成、客户端指定、包含空格的非法 ID | 三次均 HTTP 200；自动 ID `bb4005a5-6a45-4d9a-92b7-a779f82dda13`，合法指定值原样返回，非法值被替换为 UUID | 通过 | `docs/test-evidence/server.stdout.log` |
| 上传 `POST /projects/upload` | `flash-sale-prguard-package.zip`，31,019 bytes | HTTP 200，project_id=8，21 files；响应未返回 `code_version_hash`，数据库实值为 `f2478e27...10146a` | 通过；记录响应字段限制 | `docs/test-evidence/server.stdout.log` |
| Index | project_id=8，连续执行两次 | 两次均得到 69 chunks（Go 48/Text 21）、69 embeddings、69 upserts；MySQL 最终仍为 69，不重复累加 | 通过（计数） | `docs/test-evidence/server.stdout.log` |
| Diff 上传 | UTF-8 unified diff，296 bytes | HTTP 200，diff_id=6，diff_hash=`4ebfe733a71237df83c87bff193c085a11074afb12ebfe5a099f13aae131ef12`，1 个修改文件 | 通过 | `docs/test-evidence/flash-sale-day21.diff` |
| diff 归属校验 | project_id=7 搭配 diff_id=6 | HTTP 400，`diff does not belong to project` | 通过 | `docs/test-evidence/server.stdout.log` |
| Retrieve（稳定数据） | project_id=6、diff_id=5、top_k=5 | 返回 5 个 related files、5 个 symbols、5 个 context chunks | 通过 | `docs/test-evidence/server.stdout.log` |
| Retrieve（重复 Index 后） | project_id=8、diff_id=6、top_k=5 | HTTP 500：`code_chunk not found: 542`；旧 Qdrant 点仍引用已被重建的 MySQL Chunk ID | **失败，发现真实缺陷** | `docs/test-evidence/server.stdout.log` |
| 同步 Analyze 与缓存 | 清理限定 Key 后，相同 top_k 连续请求 | 第一次 `cached=false`，第二次 `cached=true`；E2E 脚本全部 PASS | 通过 | DAY21 E2E 终端输出；`server.stdout.log` |
| top_k 缓存隔离 | top_k=5 与 8 | 两个真实 Redis Key 均带 `topk`；首次互不命中，TTL 均为 3600 秒；Redis JSON 内 `cached=false` 被正确视为合法 | 通过 | DAY21 cache 脚本终端输出 |
| LLM markdown JSON | `mock_mode=markdown_json`、cache disabled | degraded=false、report_id=27；`risk_reports` 数量 9 -> 10 | 通过 | `docs/test-evidence/server-markdown.log` |
| Fallback invalid_json | `mock_mode=invalid_json`、cache disabled | degraded=true、reason=`llm_invalid_json`、report_id=0；报告数 10 -> 10，缓存 Key 数 0 -> 0 | 通过 | `docs/test-evidence/server-invalid-json.log` |
| Fallback timeout | delay=3000ms、LLM timeout=1s | 约 1318ms 返回；degraded=true、reason=`llm_timeout`、report_id=0、不写缓存 | 通过 | `docs/test-evidence/server-timeout.log` |
| 来源白名单 | `mock_mode=invented_source` | degraded=true、reason=`llm_invalid_report`；虚构文件未进入结果，报告数不变 | 通过 | `docs/test-evidence/server-invented-source.log` |
| 固定窗口限流 | limit=3/60s，连续 5 次 Analyze | 状态码依次 200、200、200、429、429；第四次 `Retry-After=60`；Redis count=5、TTL=59 | 通过 | `docs/test-evidence/server-rate-limit.log` |
| Redis fail-open | cache disabled；临时停止 Redis | Analyze 仍 HTTP 200、degraded=false；日志出现 `rate_limit_redis_error` 和 `fail_open=true`；测试后 Redis PONG | 通过 | `docs/test-evidence/server-rate-limit.log` |
| 异步任务 | delay=1000ms；top_k=13/14 | 提交耗时 126ms；task 18/19 经 running 后 succeeded，attempt=1 | 通过 | DAY21 async 脚本终端输出 |
| 幂等提交 | 重复提交 top_k=13 | 第二次返回 task_id=18、`reused=true`；不同 top_k 返回 task_id=19 | 通过 | DAY21 async 脚本终端输出 |
| Worker 并发 | worker_count=2，同时执行两个延迟任务 | 最大观测 busy_worker_count=2，未超过 configured_worker_count=2 | 通过 | DAY21 async 脚本终端输出 |
| Qdrant 恢复后重试成功 | task 20/top_k=18；显式停止后恢复 Qdrant | attempt 1 为 pending，`qdrant_unavailable` 且有 next_run_at；恢复后 attempt 2 succeeded，report_id=32 | 通过 | `docs/test-evidence/server-retry.log` |
| 重试耗尽 | task 21/top_k=19；Qdrant 保持不可用 | attempt_count=3=max_attempts，最终 failed、error_code=`retry_exhausted`、next_run_at=null | 通过 | `docs/test-evidence/server-retry.log` |
| 异步 Fallback | task 22/top_k=20；invalid_json | attempt 1 succeeded、degraded=true、report_id=null，不触发重试 | 通过 | `docs/test-evidence/server-fallback-task.log` |
| stale recovery | task 23/top_k=17；running 时强制终止进程，等待超过临时 stale=10s 后重启 | 启动日志 recovered_count=1；旧 worker 的 attempt 1 被重新排期；新 worker attempt 2 succeeded、report_id=33 | 通过 | `server-stale-before-crash.log`、`server-stale-after-restart.log` |
| Ops task list | 分页、status/project/error_code 过滤、非法 page | total=7；failed 过滤返回 task 21；page=0 返回 400；响应无 `result_json`、`task_key`、`result` | 通过 | `docs/test-evidence/server-final-normal.log` |
| Ops metrics | 24 小时窗口并用 SQL 核对 | API 与 SQL 均为 submitted=7、succeeded=6、failed=1、degraded succeeded=1、retried=3；错误分布 retry_exhausted=1 | 通过 | `docs/test-evidence/server-final-normal.log` + 本次终端 SQL 输出 |
| Worker Runtime | 服务空闲时查询 | configured=2、registered=2、busy=0、idle=2，workers 数组一致 | 通过 | `docs/test-evidence/server-final-normal.log` |
| Ops disabled | 临时 `ops.enabled=false` 后重启 | `GET /ops/workers` 返回 HTTP 404；随后恢复 true | 通过 | `docs/test-evidence/server-ops-disabled.log` |
| 数据库最终状态 | 按 id 倒序查询最后 20 个 analysis_tasks | 共 7 条：6 succeeded、1 failed；Fallback task 22 为 degraded=1/report_id=NULL/attempt=1，重试成功 task 20/23 为 attempt=2，耗尽 task 21 为 attempt=3/max=3/retry_exhausted | 通过 | 本次终端 MySQL 输出 |
| Redis 最终状态 | 分别 `--scan --pattern` 报告/限流 Key，并查询 INFO keyspace | 6 个报告 Key，均为 project 6 且 top_k 分别 5/8/11/12/13/14；限流 Key 已过期为空；db0 keys=6、expires=6、avg_ttl=2008350ms | 通过 | 本次终端 redis-cli 输出；未使用 `KEYS *` 或 FLUSHDB |
| 优雅关闭 | 空闲及运行中任务分别发送 Ctrl+C/SIGTERM | 未执行：当前 Windows 自动化环境无法安全地只向独立服务进程发送 Ctrl+C 并保证不影响宿主会话 | **需要人工验证** | 按 README 启动后人工 Ctrl+C，检查 `server_shutdown_started/completed` 与 worker 日志 |

## 代码质量命令

| 命令 | 实际结果 | 退出码 | 结论 |
|---|---|---:|---|
| `go fmt ./...` | 默认系统 Go Cache 被沙箱拒绝；设置工作区 `GOCACHE=.gocache` 后无输出 | 0 | 通过 |
| `go vet ./...` | 设置工作区 GOCACHE 后无输出 | 0 | 通过 |
| `go test ./...` | 所有列出的 package 均 `ok` 或 `[no test files]` | 0 | 通过 |
| `go test -race ./internal/worker/...` | 默认 CGO 未开启；设置 `CGO_ENABLED=1` 后报 `cgo: C compiler "gcc" not found` | 1 | **未通过（环境缺少 GCC）** |

## 脚本与压测执行记录

| 项目 | 参数摘要 | 实际结果 | 结论 |
|---|---|---|---|
| `scripts/day21-e2e.ps1` | project=6、diff=5、top_k=11、alternate=12 | 全部 PASS；异步 task_id=17 succeeded | 通过 |
| `scripts/day21-cache-test.ps1` | project=6、diff=5、top_k=5/8；限定 Key 删除 | 首次 false、第二次 true、TTL=3600、Key 隔离；未使用 FLUSHDB | 通过 |
| `scripts/day21-async-test.ps1` | project=6、diff=5、top_k=13/14、max submit=800ms | 126ms 返回；幂等和 Worker 并发检查 PASS | 通过 |
| `scripts/day21-fault-test.ps1` | 未整套直接运行；按相同场景逐个手工控制配置/容器 | invalid JSON、timeout、Qdrant retry、Redis fail-open、retry exhausted 均取得上述真实结果 | 场景通过；脚本入口本身需要人工复跑 |
| 开发基线 | 直接按总流程使用 hey 的五组指定 n/c | 五组均 HTTP 200、失败 0；真实 RPS/延迟见压测报告 | 通过 |

压测详情：[benchmark-results.md](benchmark-results.md)。服务日志与测试 diff 位于 `docs/test-evidence/`；部分 API/Redis/MySQL 断言只存在本次 Codex 终端输出，未伪造截图路径。

## 安全检查

- `git ls-files -- .env .env.*` 无输出；`.gitignore` 忽略 `.env` 和运行时 `data/projects/`。
- `config.example.yaml` 只含环境变量占位符；运行配置的 API Key 为空，MySQL 密码通过环境变量注入。
- Go 日志调用扫描未发现记录完整源码、diff、Prompt、Authorization、API Key 或 Provider 原始响应的字段。
- Ops 列表实测不返回 ResultJSON、task_key 或完整结果；客户端响应未观察到 DSN、Redis 密码或内部堆栈。
- README 已说明 ZIP 20 MiB、diff 50 MiB、现有路径穿越保护及 ZIP bomb 配额不足；Ops 明确仅建议开发/受控内网。

## 未通过项、已知问题与人工项

1. **重复 Index 后 Retrieve 失败**：project_id=8 可稳定复现旧 Qdrant 点引用已删除 MySQL Chunk ID。DAY21 要求禁止新增业务能力和大规模重构，本次只记录，没有修改索引业务语义。
2. **Race 测试环境不完整**：需要安装可供 Go CGO 使用的 GCC/MinGW-w64 后重新执行；当前不能宣称通过。
3. **优雅关闭需要人工验证**：应在独立终端运行服务，分别于空闲和长任务运行期间按 Ctrl+C，保存 shutdown/worker 日志。
4. **真实 Provider 需要人工验证**：本次未使用真实 Embedding/LLM，不对准确率、召回质量或外部 API 性能作结论。
