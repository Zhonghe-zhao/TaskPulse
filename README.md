# TaskPulse

TaskPulse 是一个基于 Go 开发的可靠异步任务执行系统。

这个项目用于学习和解决长耗时、批量任务背后的后端工程问题：

- 有界并发
- 任务状态机
- 超时与取消
- 重试与幂等
- 进程崩溃后的任务恢复
- 任务进度追踪
- 可观测性与性能分析

TaskPulse 第一阶段不会把自己包装成万能工作流平台。我们先通过真实任务验证系统，再根据实际遇到的问题抽象通用基础设施能力。

项目的正式问题定义、边界和完成标准见：

- [文档索引](docs/README.md)
- [TaskPulse 项目章程](docs/PROJECT_CHARTER.md)
- [最小可行版本计划](docs/MVP.md)
- [系统架构](docs/ARCHITECTURE.md)
- [学习路径](docs/STUDY_PATH.md)

## 第一个真实场景：URL Inspector

TaskPulse 的第一个上层应用是“批量 URL 检测与网页元数据采集”。

用户提交一批 URL，系统执行：

```text
创建批次任务
→ 拆分为多个 URL 子任务
→ Worker 并发访问 URL
→ 记录状态码、响应时间、重定向、网页标题和错误
→ 对临时错误进行重试
→ 持续更新任务进度
→ 保存最终检测报告
```

这个场景会自然产生 TaskPulse 需要解决的问题：

- 每个网络请求的耗时不可预测
- 部分请求可能超时
- 有些错误可以重试，有些错误不能重试
- 必须限制并发数量，避免耗尽资源
- 单个 URL 失败不能导致整个批次失败
- 用户需要查看批次进度和每个 URL 的结果
- Worker 中断后，未完成任务需要恢复

## 项目边界

项目包含两层：

```text
TaskPulse Core
  负责任务生命周期、存储、排队、Worker、重试、租约、事件和监控

URL Inspector
  负责 URL 校验、HTTP 请求、网页元数据提取和结果展示

LLM Analysis
  负责笔记分析输入校验、调用可替换 LLM Client、输出结构化分析结果
```

URL Inspector 用于证明 TaskPulse Core 能处理网络批处理问题。LLM Analysis 用于证明同一套 Core 能承载受限流、长耗时和成本敏感的智能任务。

TaskPulse Core 中不能出现 URL 特有的业务规则。

MemoBridge 不是 TaskPulse 第一版的依赖。未来 MemoBridge 真正出现批量链接检测、大型导出或 LLM 批处理需求时，再考虑接入。

## 本地运行

真实运行默认使用 MySQL。`compose.yaml` 第一次创建数据卷时会自动执行初始化迁移。

```powershell
docker compose up -d
```

应用直接读取进程环境变量，不会自动加载 `.env.example`。启动前需要在当前 PowerShell 设置 `MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_DATABASE` 等变量，然后运行：

```powershell
$env:TASKPULSE_STORAGE="mysql"
go run ./cmd/taskpulse
```

服务启动日志必须出现：

```text
TaskPulse storage backend: mysql
TaskPulse HTTP server listening on :8080
```

默认启动 1 个任务级 Worker。需要验证任务级并发时，可以设置：

```powershell
$env:TASKPULSE_WORKER_COUNT="4"
```

每个 Worker 使用独立的 `worker_id`，共享 MySQL 任务队列；任务领取由事务和
`FOR UPDATE SKIP LOCKED` 保证，不会因为启动多个 Worker 就重复有效领取同一个任务。

多进程实验时，可以为不同进程设置不同的 HTTP 地址，例如 `:8080` 和 `:8081`；两个进程仍然连接同一个 MySQL 任务队列。

只有单元测试或不需要持久化的临时调试才显式使用：

```powershell
$env:TASKPULSE_STORAGE="memory"
go run ./cmd/taskpulse
```

MySQL 连接失败时应用直接退出，不会静默退回内存存储。

## 当前 HTTP API

```text
POST /tasks
GET  /tasks/{task_id}
GET  /tasks/{task_id}/events
POST /tasks/{task_id}/cancel
GET  /metrics
```

外部 Worker 协议：

```text
POST /worker/tasks/claim
POST /worker/tasks/{task_id}/heartbeat
POST /worker/tasks/{task_id}/complete
POST /worker/tasks/{task_id}/fail
```

领取任务：

```json
{
  "worker_id": "memobridge-worker-1",
  "lease_duration": "30s"
}
```

完成任务时必须携带领取响应中的 `version`：

```json
{
  "worker_id": "memobridge-worker-1",
  "version": 1,
  "output": {
    "summary": "..."
  }
}
```

`version` 用于阻止旧 Worker 在任务被恢复、取消或被其他 Worker 接管后覆盖结果。
第一版外部 Worker 的失败接口直接记录最终失败；统一的外部重试协议将在本协议稳定后补充。

取消接口接受 `queued`、`retrying` 和 `running` 任务。重复取消是幂等操作。
运行中的任务会先被原子标记为 `canceled` 并清除租约，Worker 通过续租失败取消
Executor Context；旧 Worker 不能再提交成功结果。

## LLM Analysis 演示

`llm_analysis` 当前使用 Fake LLM Client，不会调用真实模型。它用于验证 TaskPulse 的
workflow 注册、Worker 执行、结果持久化和事件链路。

创建任务：

```powershell
$body = @{
  workflow = "llm_analysis"
  input = @{
    subject = "Go concurrency"
    notes = @("goroutine", "channel", "context")
    goal = "make a two week study plan"
  }
  max_retries = 3
} | ConvertTo-Json -Depth 5

Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8080/tasks `
  -ContentType "application/json" `
  -Body $body
```

返回结果中的 `id` 是后续查询使用的 `task_id`。

查询任务：

```powershell
Invoke-RestMethod -Method Get -Uri http://localhost:8080/tasks/{task_id}
```

查询事件：

```powershell
Invoke-RestMethod -Method Get -Uri http://localhost:8080/tasks/{task_id}/events
```

期望看到：

```text
task.status = succeeded
task.result.subject = Go concurrency
task.result.model = fake-llm
events = task_created → task_started → task_succeeded
```

如果任务长时间停留在 `queued`，优先检查应用启动日志中是否有 Worker 错误，以及当前
`TASKPULSE_STORAGE` 对应的存储是否已经初始化。

模拟一次 LLM 限流后成功：

```powershell
$env:TASKPULSE_LLM_FAKE_FAILURE="rate_limited_once"
go run ./cmd/taskpulse
```

再次提交上面的 `llm_analysis` 请求后，期望事件序列变为：

```text
task_created
task_started
task_retrying
task_retry_started
task_succeeded
```

这个模拟只属于 Fake Client，用来验证 TaskPulse 的通用重试链路。真实模型接入后，
限流和上游故障应由真实 Provider Client 根据 HTTP 状态码和超时错误映射。

## 独立 LLM Worker 演示

TaskPulse 也提供一个独立的外部 Worker 示例。启动 TaskPulse API 和 MySQL 时先关闭内部 Worker：

```powershell
$env:TASKPULSE_INTERNAL_WORKERS_ENABLED="false"
go run ./cmd/taskpulse
```

再在另一个终端运行：

```powershell
$env:TASKPULSE_URL="http://localhost:8080"
$env:TASKPULSE_EXTERNAL_WORKER_ID="external-llm-worker-1"
$env:TASKPULSE_EXTERNAL_LEASE="30s"
$env:TASKPULSE_LLM_FAKE_DELAY="3s"
go run ./cmd/llm-worker
```

随后通过 `POST /tasks` 创建 `llm_analysis` 任务。独立 Worker 会通过 HTTP 领取任务、续租、执行 Fake LLM 并提交结果。TaskPulse 进程不直接执行这个任务。

## Worker 崩溃恢复演示

为了稳定复现长任务和租约恢复，可以设置：

```powershell
$env:TASKPULSE_LLM_FAKE_DELAY="10s"
$env:TASKPULSE_WORKER_LEASE_DURATION="5s"
```

启动第一个进程、提交 `llm_analysis` 任务，并在任务处于 `running` 时终止进程。
租约到期后启动第二个进程，任务应被重新领取并最终完成。完整步骤见
`docs/experiments/06-worker-crash-recovery.md`。

## 可观测性

TaskPulse 使用结构化文本日志记录 Worker 和 Reaper 的关键动作。常见日志事件：

```text
task claimed                  INFO
task succeeded                INFO
task partially succeeded      WARN
task failed                   ERROR
task scheduled for retry      WARN
task lease renewed            DEBUG
task lease renewal failed     WARN
expired task failed by reaper WARN
```

本地终端使用彩色人类可读格式：成功为绿色，警告为黄色，错误为红色。日志仍然保留
`task_id`、`workflow`、`worker_id` 等结构化字段，便于后续被日志系统采集。

关键字段包括：

```text
task_id
workflow
worker_id
status
retry_count
duration_ms
error_code
available_at
```

Prometheus 文本格式指标通过 `/metrics` 暴露：

```powershell
Invoke-RestMethod -Method Get -Uri http://localhost:8080/metrics
```

当前指标：

```text
taskpulse_tasks_claimed_total{workflow}
taskpulse_tasks_completed_total{workflow,status}
taskpulse_tasks_retried_total{workflow,error_code}
taskpulse_lease_renewed_total{workflow}
taskpulse_lease_lost_total{workflow}
taskpulse_reaper_expired_failures_total{workflow}
taskpulse_tasks_current{status}
taskpulse_tasks_available_current{status}
taskpulse_oldest_available_task_age_seconds{status}
taskpulse_task_execution_duration_seconds_bucket{workflow,le}
taskpulse_task_execution_duration_seconds_sum{workflow}
taskpulse_task_execution_duration_seconds_count{workflow}
```

这些指标用于回答：

```text
任务有没有被领取？
任务最终成功、失败、部分成功分别有多少？
哪些错误触发了重试？
当前各状态任务堆积了多少？
有多少 queued/retrying 任务已经可领取？
最老的可领取任务等待了多久？
租约是否正常续期？
是否出现 Worker 丢失租约？
任务执行耗时分布如何？
```

## 开发演进路线

```text
内存存储和内存队列
→ MySQL 8 持久化
→ 使用 FOR UPDATE SKIP LOCKED 实现 MySQL 任务领取
→ 加入租约和崩溃恢复
→ 加入指标、故障测试和压力测试
→ 根据测试结果判断是否需要 Redis Stream
```

## 第一版不做什么

- 可视化工作流编辑器
- 动态工作流 DSL
- 多 Agent 协作
- 插件市场
- 为展示技术而强行加入 Kafka
- 在单机系统尚未测量前引入 Kubernetes
- 强行接入 MemoBridge
