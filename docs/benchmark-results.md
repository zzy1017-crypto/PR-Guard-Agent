# PR-Guard-Agent 开发环境性能基线

本报告是 2026-07-17 在本地开发环境、Mock Provider 下测得的真实基础数据，不代表生产性能，也不代表真实 LLM 的吞吐或延迟。Analyze 在测试前已验证 `cached=true`，没有对真实 LLM API 发起高并发请求。

## 测试环境

| 项目 | 实际值 |
|---|---|
| 时间 | 2026-07-17 15:55–15:57（Asia/Shanghai） |
| Git Commit | `eca107bf4e844a39eda3755e435c66b8ce43dfd7`（测试时工作区有未提交 DAY21 修改） |
| 操作系统 | Microsoft Windows NT 10.0.26200.0，64 位 |
| CPU | Intel64 Family 6 Model 170 Stepping 4；18 个逻辑处理器（环境变量/Go Runtime 可见值） |
| 内存 | 未取得：沙箱账户访问 Win32 CIM 被拒绝，需要人工补充 |
| Go | `go1.25.4 windows/amd64` |
| hey | `C:\Users\Lenovo\go\bin\hey.exe`；该构建不支持 `--version` |
| Provider | Embedding `mock/mock_embedding`；LLM `mock/mock-llm`，`mock_mode=normal` |
| Analyze 缓存 | project_id=6、diff_id=5、top_k=5；预热响应 `cached=true`，report_id=21 |
| 临时配置 | 压测期间 `rate_limit.limit=10000`、窗口 60 秒；测试完成后恢复为 100 |

## 实测汇总

| 接口 | requests | concurrency | 总耗时 | RPS | 平均 | P50 | P90 | P95 | P99 | 成功 | 失败 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `GET /health` | 1000 | 50 | 0.2175 s | 4597.8682 | 10.5 ms | 9.5 ms | 11.9 ms | 22.9 ms | 33.2 ms | 1000 | 0 |
| `POST /projects/6/diffs/5/analyze?top_k=5`（缓存命中） | 200 | 10 | 0.3167 s | 631.5972 | 14.8 ms | 12.1 ms | 19.3 ms | 33.1 ms | 55.0 ms | 200 | 0 |
| `GET /analysis-tasks/17` | 500 | 20 | 0.3283 s | 1522.7946 | 11.8 ms | 7.4 ms | 18.0 ms | 35.7 ms | 96.4 ms | 500 | 0 |
| `GET /ops/workers` | 500 | 20 | 0.1410 s | 3546.6703 | 5.5 ms | 4.8 ms | 5.8 ms | 6.1 ms | 25.4 ms | 500 | 0 |
| `GET /ops/analysis-tasks/metrics?window_hours=24` | 200 | 10 | 0.3170 s | 630.9893 | 14.8 ms | 14.2 ms | 20.6 ms | 31.6 ms | 49.3 ms | 200 | 0 |

成功/失败依据 `hey` 的 HTTP 状态码分布：以上五组均只有 HTTP 200。百分位来自 `hey` 的 Latency distribution；`hey` 不直接打印 P50/P90/P95/P99 标签，表中是对应 50%/90%/95%/99% 行的真实值。

## 实际命令

```powershell
hey -n 1000 -c 50 http://localhost:8080/health
hey -n 200 -c 10 -m POST -H "Content-Type: application/json" -d "{}" "http://localhost:8080/projects/6/diffs/5/analyze?top_k=5"
hey -n 500 -c 20 http://localhost:8080/analysis-tasks/17
hey -n 500 -c 20 http://localhost:8080/ops/workers
hey -n 200 -c 10 "http://localhost:8080/ops/analysis-tasks/metrics?window_hours=24"
```

## 结果边界

- 数据只反映该次本机短时请求的开发基线，没有进行持续稳定性、容量上限、网络抖动或多实例测试。
- Analyze 是 Redis 缓存命中路径；不能用其 RPS 推断未命中检索、Embedding 或 LLM 路径的性能。
- Mock Provider 不代表真实语义效果或外部 Provider 延迟。
- 服务访问日志会增加少量 I/O；本次没有单独控制后台进程和系统负载。
