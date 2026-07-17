# PR-Guard-Agent 面试复盘

回答边界：以下内容只描述当前仓库已实现能力。Mock Provider 只证明工程链路，不代表真实模型准确率或生产吞吐。

## 1 分钟项目介绍

PR-Guard-Agent 是一个 Go PR 变更风险分析后端。用户上传 Go 项目 ZIP 和 Git diff，系统先过滤文件并计算源码版本哈希，再用 Go AST 把源码按函数、方法、结构体等语义单元分块；非 Go 文件使用重叠文本窗口。代码块经过 Embedding 写入 Qdrant。分析 diff 时，系统按项目和源码版本做 TopK 检索，把相关文件、符号和代码块组成 RAG 上下文，再调用 LLM 生成结构化风险报告，并对 JSON 字段和来源白名单二次校验。同步接口用 Redis 缓存和限流；异步接口把任务持久化到 MySQL，固定 Worker Pool 用 `SKIP LOCKED` 并发领取，支持幂等、超时、Fallback、指数退避、重试耗尽和 stale 恢复。项目还提供 Request ID、Zap 日志、任务指标与 Worker Runtime。

## 3 分钟项目介绍

这个项目解决的是“只看 diff 缺少上下文”的问题。入口数据有两类：源码 ZIP 和 unified Git diff。上传源码时限制 20 MiB，过滤扩展名和目录，检查 ZIP 路径穿越；每个文件有内容哈希，项目有 `code_version_hash`。它既用来隔离向量数据，也参与缓存和任务身份，避免代码更新后复用旧结果。

索引阶段对 Go 文件使用 `go/parser`。函数、方法、struct、interface、const、var 分别形成代码块，保留符号、行号、内容哈希和版本；Markdown、YAML、JSON、SQL、Lua、go.mod、go.sum 使用 1500 rune、200 overlap 的文本块。Embedding 分批生成，Qdrant payload 保存项目、版本、chunk、文件和符号信息。

分析阶段先检查 project/diff 归属和 Redis Key。如果未命中，系统对 diff 生成查询向量，在 Qdrant 强制过滤 `project_id + code_version_hash`，再回 MySQL加载原文并校验归属。Prompt 包含 diff 和受限上下文。LLM 输出必须通过 JSON schema 式字段校验、值域校验和 related file/symbol 白名单校验。正常结果写 MySQL 和 Redis；LLM 超时、Provider 错误、非法 JSON 或来源错误走 `degraded=true` 的 Fallback，而且不会污染正式报告表或缓存。

异步提交不在 Handler 启 goroutine，而是写 `analysis_tasks`。任务 Key 对项目版本、diff hash、top_k 做 SHA-256，数据库唯一索引保证并发幂等。固定数量 Worker 在事务里用 MySQL 8 `FOR UPDATE SKIP LOCKED` 领取任务。每次状态更新都带当前 status、worker_id 和 attempt_count，防止旧 Worker 覆盖新 Worker。临时错误指数退避并加 jitter，永久错误直接失败，达到最大次数标记 `retry_exhausted`。启动时扫描 stale running 任务恢复。最后用 Request ID、Zap、Redis Lua 限流、Ops 指标和 Worker 快照支持排查。

## 为什么使用 Go

项目本身分析 Go 代码，标准库直接提供 `go/parser`、`go/ast`、`go/token`，不需要额外语言服务就能得到可靠语法边界。服务端同时涉及 HTTP、外部 IO 和固定并发 Worker，Go 的 Context、goroutine 和类型系统适合实现可控并发与超时。选择 Go 是工程适配，不等于 Go 在所有 AI 服务场景都优于其他语言。

## 为什么使用 AST 而不是固定长度切块

固定长度可能把函数签名和函数体、方法和接收者切开。AST 能让函数、方法、struct、interface、const、var 保持声明级完整，并给出准确行号和符号类型，有利于检索结果解释和来源校验。非 Go 文本没有统一 Go AST，所以当前仍采用 1500 rune、200 overlap 的折中方案。

## 为什么需要 `code_version_hash`

同一个 `project_id` 会随重新上传或源码变化产生不同代码语义。版本哈希进入代码块、Qdrant Filter、报告 Cache Key 和任务 Key，防止新 diff 检索旧向量、命中旧报告或复用旧任务。它把“项目身份”和“源码快照身份”分开。

## Qdrant payload 和过滤条件

payload 包含 `project_id`、`code_version_hash`、`chunk_id`、`file_path`、`symbol_name`、`symbol_type`、`start_line`、`end_line`、`content_hash`。查询必须同时匹配项目 ID 和版本哈希；命中后服务还会回 MySQL 加载 chunk，并再次检查 ProjectID 和 CodeVersionHash。

## RAG 如何减少幻觉

RAG 把完整 diff 映射为查询向量，只给 LLM TopK 相关代码块，而不是要求模型凭空判断整个仓库。每块上下文带文件、符号、行号和分数；单块文本限制长度。最终 `related_files` 和 `related_symbols` 必须来自上下文白名单。它降低无依据引用概率，但不能消除错误推理，也不能证明风险等级准确。

## 为什么 LLM 输出必须再次校验

LLM 即使被要求输出 JSON，也可能返回 Markdown fence、非法 JSON、缺字段、越界 confidence 或虚构来源。当前代码会提取并解析 JSON，校验风险等级、非空字段、数组、confidence 范围，再校验文件和符号来源。非法结果进入 Fallback，而不是直接入库。

## Cache Key 为什么包含 `top_k`

`top_k` 改变 RAG 上下文，进而改变 Prompt 和报告。若 Key 不包含它，top_k=5 的报告可能被 top_k=8 请求复用，缓存语义不正确。当前格式是 `project_id + code_version_hash + diff_hash + top_k`。

## 为什么 Fallback 不写正式缓存

Fallback 表示模型结果不可用，只是让调用方得到可识别的降级响应。如果写入 `risk_reports` 或 Redis，短暂故障会被当作正式报告长期复用，恢复后也无法自动得到正常分析。当前 Fallback 的 `report_id=0`、`cached=false`、`degraded=true`，异步任务可以 succeeded，但不污染正式报告数据。

## 为什么使用 Request ID 和结构化日志

一次请求会跨 Handler、Redis、RAG、LLM、MySQL，异步任务还会跨越提交和 Worker 执行。Request ID 让同步链路可关联；异步执行使用 `analysis-task-<id>` 的独立上下文标识。Zap 字段化记录项目、diff、task、worker、top_k、状态和耗时，便于过滤与统计，同时避免打印源码、diff、Prompt 和凭证。

## Redis Lua 限流为什么原子

固定窗口需要 `INCR`、首次 `EXPIRE` 和读取 `TTL` 成为一个不可插入的操作。若客户端分多次命令，进程崩溃或并发交错可能产生无 TTL Key 或错误计数。当前 Lua 脚本在 Redis 单线程执行模型下作为一个原子单元完成三步，并返回计数和剩余窗口。

## 为什么 Analyze 异步化

Embedding、Qdrant 和 LLM 都可能慢或暂时失败，同步请求容易占用连接并受客户端断开影响。异步任务让提交快速返回 task_id，把执行、重试和状态持久化交给 Worker；同步接口仍保留，适合低延迟或直接调用场景。

## 为什么不在 Handler 中启动 goroutine

Handler goroutine 没有持久化队列，进程重启会丢任务；并发量跟请求量增长，难以限流；HTTP Context 结束后还可能被取消；错误和结果也难追踪。当前 Handler 只写 MySQL 任务，固定 Worker Pool 控制并发并使用独立 Context。

## 为什么使用 MySQL `SKIP LOCKED`

多个 Worker 同时找 pending 任务时，普通锁可能互相等待。`FOR UPDATE SKIP LOCKED` 让每个事务跳过其他 Worker 已锁定的行，领取不同任务。领取和 pending->running 更新在同一事务中，适合已有 MySQL 基础设施的持久任务队列；它不是在声称 MySQL 等同专业消息队列。

## 如何保证任务幂等

任务身份对 `project_id`、`code_version_hash`、`diff_hash`、`top_k` 做长度消歧拼接后 SHA-256。`task_key` 有唯一索引；提交先查询，创建遇到并发唯一键冲突后再查一次。pending、running、succeeded 会复用同一 task_id；未耗尽的 failed 可重置，已耗尽返回冲突。

## 如何区分可重试与永久错误

`taskerror.Classify` 按 `errors.Is/As` 分类。Embedding/Qdrant 临时错误、任务超时、部分 MySQL 死锁/超时/连接错误可重试；项目或 diff 不存在、归属错误、空 diff、非法 top_k、非法任务数据/状态属于永久错误。未知错误当前保守归为 `internal_analysis_error` 的永久失败，避免无界重试。

## 为什么使用指数退避和 jitter

依赖故障时立即固定频率重试会继续施压，并让多个任务同一时刻形成惊群。当前延迟从 `retry_base_seconds` 按 attempt 翻倍，封顶 `retry_max_seconds`，再加入最多配置比例的正负随机 jitter，最终仍限制在合法范围。

## 如何避免旧 Worker 覆盖新 Worker

任务成功、重新排期和最终失败更新都带 `id + status=running + worker_id + expected attempt_count`。stale 恢复还额外检查旧 started_at。若任务已被恢复并被新 Worker 领取，旧 Worker 的条件更新影响 0 行并返回状态冲突，不会覆盖新状态。

## 如何监控队列积压

`/ops/analysis-tasks/metrics` 提供 pending、到期 pending、scheduled retry、running、stale running、最老 pending 年龄，以及窗口内提交、成功、失败、降级、重试、平均排队和运行时长。`/ops/workers` 提供配置/注册/忙闲 Worker、当前 task 和成功失败计数。当前是查询接口和进程内快照，尚未接 Prometheus/告警。

## 当前项目的不足和后续改进

- Ops 和开发测试路由没有认证，只适合开发或受控内网。
- Mock Provider 不验证真实检索质量、报告准确率或生产性能，需要离线评测集。
- ZIP 已限制请求大小和路径穿越，但缺少解压总量、压缩比、文件数及单文件配额。
- MySQL 任务队列没有优先级、取消、死信队列和跨进程心跳；Runtime 指标重启即清零。
- 当前只有 Zap 和查询型 Ops，没有 Prometheus、OpenTelemetry 和自动告警。
- 生产数据库迁移、数据保留、`risk_reports.RawJSON` 权限与 Provider 合规仍需部署层治理。
- 需要为真实 Embedding 维度迁移、Qdrant Collection 版本和模型升级建立显式流程。
