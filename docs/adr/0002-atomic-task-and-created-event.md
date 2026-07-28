# ADR-0002：原子创建任务与初始事件

- 状态：已实现
- 日期：2026-07-27
- 决策者：TaskPulse 项目维护者

## 背景

创建任务用例原先依次调用：

```text
TaskStore.Create
→ EventStore.Append(task_created)
```

如果任务写入成功而事件写入失败，数据库中会存在没有初始事件的任务。Application 无法通过补偿删除可靠地解决该问题，因为删除补偿也可能失败，并且并发读取可能在补偿前观察到不完整状态。

## 决策

新增 `TaskCreationStore` 端口：

```go
CreateTaskWithEvent(ctx, task, event) error
```

Application 负责表达“Task 与 Created Event 必须一起创建”，但不知道具体事务技术：

- Memory 实现同时锁定 Task Store 与 Event Store，完成检查后再写入两份数据；
- MySQL 实现使用同一个 `sql.Tx` 插入 `tasks` 与 `task_events`，任一步失败都回滚；
- 单独的 `TaskStore.Create` 和 `EventStore.Append` 仍然保留，供其他明确只写一种数据的场景使用。

## 备选方案

### Application 先写 Task，再写 Event

无法保证原子性，已放弃。

### Event 写入失败后删除 Task

补偿操作本身可能失败，并且存在中间状态可见窗口，暂不采用。

### Outbox

当前 Task 与 Event 位于同一个 MySQL 实例，不需要跨数据库或消息系统。Outbox 留到未来向 Redis Streams 或 Kafka 发布事件时再评估。

### 分布式事务

当前不存在多个事务资源，复杂度没有依据。

## 代价与风险

- Application 增加一个专用于创建用例的持久化端口；
- Memory 实现必须遵守固定锁顺序；
- 当前只保证 Task 与初始 Created Event 原子创建，Worker 状态更新与运行事件仍然是两次写入，尚未获得相同保证。

## 验证标准

- 正常创建后 Task 与 Created Event 同时存在；
- Event ID 冲突时，MySQL Task 插入被回滚；
- Memory 实现在 Event 冲突时不会留下 Task；
- Application 不再直接顺序调用 `TaskStore.Create` 和 `EventStore.Append`。

## 重新评估条件

当事件需要发布到 Redis Streams、Kafka 或其他外部系统时，引入 Outbox，而不是在数据库事务中直接双写外部系统。
