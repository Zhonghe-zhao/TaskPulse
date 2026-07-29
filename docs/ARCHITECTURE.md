# TaskPulse 系统架构

- 文档状态：当前架构说明
- 最近更新：2026-07-29
- 相关决策：[ADR-0001：使用 MySQL 8](adr/0001-use-mysql-as-system-of-record.md)、[ADR-0002：原子创建任务与初始事件](adr/0002-atomic-task-and-created-event.md)、[ADR-0003：原子提交任务终态与终态事件](adr/0003-atomic-terminal-task-transition.md)、[ADR-0004：原子提交任务领取与领取事件](adr/0004-atomic-task-claim-event.md)、[ADR-0005：原子提交过期任务失败与失败事件](adr/0005-atomic-expired-task-failure.md)、[ADR-0006：通用错误分类与重试语义](adr/0006-generic-retry-semantics.md)、[ADR-0007：任务创建幂等](adr/0007-idempotent-task-creation.md)、[ADR-0008：原子取消待执行任务](adr/0008-cancel-pending-tasks.md)

## 架构目标

TaskPulse 当前采用**模块化单体**：HTTP API、应用层、Worker 和执行器运行在同一个进程中，但通过接口保持模块边界。

当前阶段优先验证任务生命周期和可靠性问题，不通过拆分微服务制造复杂度。未来将 API 和 Worker 拆为独立进程时，领域模型和应用接口应尽量保持稳定。

## 当前运行架构

```text
客户端
  │
  │ POST /tasks / POST /tasks/{id}/cancel
  ▼
transport/http
  │ 解析 JSON、映射 HTTP 状态码
  ▼
application.TaskService
  │ CreateTaskWithEvent
  ▼
MySQL TaskCreationStore
  │ 同一事务写入 Task 与 task_created
  ▼
MySQL 8（Task/Event 真相源与任务队列）
  ▲
  │ ClaimNextWithEvent / RenewLease / UpdateTaskWithEvent / FailNextExpiredWithEvent
  │
worker.Worker + Reaper
  │
  ▼
Executor Registry
  │ workflow=url_check
  ▼
URLCheckExecutor
  │ 最多 5 个并发 HTTP 请求
  ▼
Result + TaskEvent
```

API 创建任务后立即返回。后台 Worker 使用 MySQL 事务原子领取任务并调用对应 Executor，通过租约心跳和版本号处理崩溃恢复与旧 Worker 隔离。Reaper 将重试额度耗尽的过期任务收敛为失败。单个进程当前只有一个任务级 Worker，因此多个 Task 之间顺序执行；单个 URL Check Task 内部最多并发检测 5 个 URL。

## 当前模块

| 模块 | 代码位置 | 职责 |
|---|---|---|
| 启动与组装 | `cmd/taskpulse` | 创建 Store、Service、Worker、Executor 和 HTTP Server |
| Domain | `internal/domain` | Task、TaskEvent、状态机和合法状态流转 |
| Application | `internal/application` | 编排创建任务、查询任务和查询事件用例 |
| Store | `internal/store`、`internal/store/mysqlstore` | 定义存储接口，提供内存测试实现和 MySQL 运行实现 |
| HTTP Transport | `internal/transport/http` | HTTP 路由、请求解析和响应映射 |
| Worker | `internal/worker` | 领取任务、选择 Executor、保存结果和终态事件 |
| URL Check Executor | `internal/executor/urlcheck` | 校验 URL、有界并发请求、汇总成功/部分成功/失败 |
| Identity | `internal/identity` | 生成进程内唯一的 Task/Event ID |
| Database Platform | `internal/platform/database` | MySQL 配置校验、连接池创建和连通性检查 |

## 依赖方向

```text
transport/http → application → domain
                         └──→ store interfaces

worker → domain
      └→ store interfaces

executor/urlcheck → worker.Executor contract
                  └→ domain.Task

store implementations → domain
```

约束：

1. HTTP Handler 不直接操作 Store。
2. Domain 不依赖 HTTP、数据库和具体 Executor。
3. URL 特有规则不能进入 TaskPulse 通用 Domain 或 Store。
4. Memory/MySQL Store 的替换不应要求修改 Handler 和 Executor。
5. Executor 返回执行结果，不直接决定任务如何持久化。

## 当前已经具备的能力

- HTTP 创建和查询任务、查询任务事件。
- Task 状态机和终态判断。
- Memory Store 的并发保护和数据深复制。
- MySQL 持久化 Task 与 TaskEvent。
- Task 与 Created Event 的事务原子创建。
- 可选 `Idempotency-Key` 的任务创建幂等、参数冲突检测和并发唯一性。
- queued/retrying 任务的幂等取消，以及取消状态与 canceled Event 的原子提交。
- Worker 终态更新与 succeeded/partial/failed Event 的事务原子提交。
- Worker 领取/恢复任务与 started/recovered Event 的事务原子提交。
- Reaper 失败清理与 failed Event 的事务原子提交。
- `FOR UPDATE SKIP LOCKED` 并发领取。
- Worker 租约、心跳续租、过期接管和版本隔离。
- Memory/MySQL Store 统一按照 `available_at` 控制任务可领取时间。
- 领取路径通过 `ClaimKind` 区分 initial、retry 和 recovery，避免根据重试次数猜测来源。
- Worker 仅对明确分类的 transient 错误应用 workflow 重试策略，普通错误和永久错误直接失败。
- 重试等待、重新领取和生命周期事件均持久化，可跨进程重启继续调度。
- 重试额度耗尽后的 Reaper 失败清理。
- Worker 根据 workflow 选择 Executor。
- URL 检测的有界并发、结果顺序保持和部分成功语义。
- Context 传递、HTTP 超时和进程信号处理。
- Domain、Store、Application、HTTP、Worker、Executor 单元测试。

## 当前明确不具备的保证

- 人工死信重放和运行中任务的协作式取消尚未完成。
- 实时事件推送尚未完成。
- SSRF 防护；当前 URL Executor 不能直接暴露到公网。
- Redis、Prometheus 或 Kubernetes 运行能力。

这些是后续要通过实现和实验获得的能力，不能在项目介绍中表述为已经完成。

## 当前持久化实现

根据 ADR-0001，下一阶段仍保持模块化单体，只增加 MySQL Store：

```text
TaskStore interface           EventStore interface
       │                              │
       ├── MemoryTaskStore            ├── MemoryEventStore
       │   用于单元测试               │   用于单元测试
       │                              │
       └── MySQLTaskStore             └── MySQLEventStore
           用于真实运行                   用于真实运行
```

当前表模型覆盖代码真实使用的 Task 与 TaskEvent：

```text
tasks
- id
- workflow
- status
- input_json / result_json
- progress
- retry_count / max_retries
- error_message
- lease_owner / lease_expires_at
- idempotency_key
- created_at / updated_at / started_at / finished_at

task_events
- id
- task_id
- type
- message
- payload_json
- progress
- created_at
```

只有当 URL 需要独立领取、独立重试和独立查询时，再通过新 ADR 引入 `task_items`，不能因为旧设计文档出现过该表就直接实现。

## 当前关键事务

### 创建任务

```text
BEGIN
→ INSERT tasks
→ INSERT task_events(task_created)
→ COMMIT
```

目标：不能出现“任务存在但创建事件不存在”。

### 领取任务

```text
BEGIN
→ SELECT queued task FOR UPDATE SKIP LOCKED
→ UPDATE status=running, lease_owner, lease_expires_at
→ INSERT task_started event
→ COMMIT
```

当前保证：多个进程竞争时只有一个 Worker 获得该任务，任务领取和 started/recovered
事件在同一事务提交，同时为崩溃恢复留下租约信息。

### 取消待执行任务

```text
BEGIN
→ SELECT task BY id FOR UPDATE
→ 校验状态为 queued/retrying
→ UPDATE status=canceled, clear lease, version=version+1
→ INSERT task_canceled event
→ COMMIT
```

当前保证：取消与 Worker 领取竞争时只有一方能够完成状态转换；重复取消返回已经取消的任务，
不会重复写入取消事件。running 任务不会被直接标记为 canceled。

### 清理重试耗尽的过期任务

```text
BEGIN
→ SELECT expired running task FOR UPDATE SKIP LOCKED
→ UPDATE status=failed, clear lease, version=version+1
→ INSERT task_failed event
→ COMMIT
```

当前保证：任务失败状态和失败事件同时提交；事件写入失败时，任务仍保持清理前状态，
可由 Reaper 在后续轮询中再次处理。

## 后续演进条件

- Redis Streams：MySQL 队列基线完成，并测得轮询/锁竞争问题或需要独立消费组后再决策。
- Prometheus：出现持久化 Worker 后引入，观测积压、等待时间、吞吐、错误和重试。
- Docker Compose：MySQL/Redis/Prometheus 等多组件出现时，用于可重复开发与测试环境。
- Kubernetes：API/Worker 已拆分、持久化和崩溃恢复完成后，用 Pod Kill 实验验证多副本行为。

## 文档同步要求

每次架构变化需要同时检查：

1. 本文档的当前运行图和能力清单。
2. `docs/MVP.md` 的里程碑状态。
3. 是否需要新增或替换 ADR。
4. 是否需要在 `docs/experiments` 增加验证证据。
