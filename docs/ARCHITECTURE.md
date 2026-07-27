# TaskPulse 系统架构

- 文档状态：当前架构说明
- 最近更新：2026-07-23
- 相关决策：[ADR-0001：使用 MySQL 8](adr/0001-use-mysql-as-system-of-record.md)

## 架构目标

TaskPulse 当前采用**模块化单体**：HTTP API、应用层、Worker 和执行器运行在同一个进程中，但通过接口保持模块边界。

当前阶段优先验证任务生命周期和可靠性问题，不通过拆分微服务制造复杂度。未来将 API 和 Worker 拆为独立进程时，领域模型和应用接口应尽量保持稳定。

## 当前运行架构

```text
客户端
  │
  │ POST /tasks
  ▼
transport/http
  │ 解析 JSON、映射 HTTP 状态码
  ▼
application.TaskService
  │ 创建 Task、记录 task_created
  ├──────────────┐
  ▼              ▼
TaskStore     EventStore
  │              │
  └──── Memory Store ────┐
                         │ 共享内存
                         ▼
                    worker.Worker
                         │ ClaimNext
                         ▼
                  Executor Registry
                         │ workflow=url_check
                         ▼
                    URLCheckExecutor
                         │ 最多 5 个并发 HTTP 请求
                         ▼
                 Result + TaskEvent
```

API 创建任务后立即返回。后台 Worker 轮询 `TaskStore`，原子领取一个 queued 任务并调用对应 Executor。当前只有一个任务级 Worker，因此多个 Task 之间顺序执行；单个 URL Check Task 内部最多并发检测 5 个 URL。

## 当前模块

| 模块 | 代码位置 | 职责 |
|---|---|---|
| 启动与组装 | `cmd/taskpulse` | 创建 Store、Service、Worker、Executor 和 HTTP Server |
| Domain | `internal/domain` | Task、TaskEvent、状态机和合法状态流转 |
| Application | `internal/application` | 编排创建任务、查询任务和查询事件用例 |
| Store | `internal/store` | 定义存储接口，提供并发安全的内存实现 |
| HTTP Transport | `internal/transport/http` | HTTP 路由、请求解析和响应映射 |
| Worker | `internal/worker` | 领取任务、选择 Executor、保存结果和终态事件 |
| URL Check Executor | `internal/executor/urlcheck` | 校验 URL、有界并发请求、汇总成功/部分成功/失败 |
| Identity | `internal/identity` | 生成进程内唯一的 Task/Event ID |
| Database Platform | `internal/platform/database` | MySQL 配置校验、连接池创建和连通性检查；当前尚未接入运行链路 |

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
- 内存 Store 的并发保护和数据深复制。
- 单进程内原子 `ClaimNext`，避免两个 goroutine 领取同一任务。
- Worker 根据 workflow 选择 Executor。
- URL 检测的有界并发、结果顺序保持和部分成功语义。
- Context 传递、HTTP 超时和进程信号处理。
- Domain、Store、Application、HTTP、Worker、Executor 单元测试。

## 当前明确不具备的保证

- 进程重启后任务不会丢失。
- 多进程 Worker 不会重复领取任务。
- Task 与 Event 原子写入。
- Worker 崩溃后的租约恢复。
- 幂等创建、自动重试、死信和任务取消。
- 持久化进度事件和实时推送。
- SSRF 防护；当前 URL Executor 不能直接暴露到公网。
- Redis、Prometheus、Docker Compose 或 Kubernetes 运行能力。

这些是后续要通过实现和实验获得的能力，不能在项目介绍中表述为已经完成。

## 下一阶段目标：MySQL 持久化

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

第一阶段表模型只覆盖当前代码真实使用的 Task 与 TaskEvent：

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

## 下一阶段关键事务

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

目标：多个进程竞争时只有一个 Worker 获得该任务，同时为崩溃恢复留下租约信息。

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
