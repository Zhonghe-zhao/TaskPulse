# ADR-0003：原子提交任务终态与终态事件

- 状态：已实现
- 日期：2026-07-28
- 决策者：TaskPulse 项目维护者

## 背景

Worker 完成 Executor 后原先依次执行：

```text
TaskStore.Update
→ EventStore.Append
```

如果任务终态更新成功而 Event 写入失败，任务会显示为成功或失败，但事件历史缺少对应终态，无法作为可靠的执行审计记录。

## 决策

新增 `TaskTransitionStore`：

```go
UpdateTaskWithEvent(ctx, task, event) error
```

- Worker 在内存中完成合法状态流转并构造终态 Event；
- Memory 实现使用固定双锁顺序，同时更新任务和追加事件；
- MySQL 实现在同一个 `sql.Tx` 中执行带版本条件的 Task 更新和 Event 插入；
- Event 冲突或插入失败时回滚 Task 更新；
- 乐观锁冲突时不写入 Event。

当前覆盖：

- `succeeded + task_succeeded`；
- `partially_succeeded + task_partially_succeeded`；
- Executor 或工作流错误导致的 `failed + task_failed`。

## 备选方案

### 更新失败后补写事件

不能修复“Task 已更新但 Event 写入失败”的窗口，放弃。

### Event 写入失败后把 Task 改回 running

补偿操作可能再次失败，并会制造非法或难以解释的状态逆转，放弃。

### 数据库触发器

事件消息、Payload 和类型属于应用语义，不应隐藏在数据库触发器中。

## 代价与风险

- Worker 增加 `TaskTransitionStore` 依赖；
- Memory Store 需要继续遵守 Task 锁在前、Event 锁在后的固定顺序；
- 当前不覆盖领取事件和 Reaper 失败事件，它们仍有双写窗口。

## 验证标准

- Task 更新与 Event 插入同时成功；
- Event ID 冲突时 Task 更新回滚；
- 版本冲突时 Event 不写入；
- Worker 不再依次调用 `TaskStore.Update` 和 `EventStore.Append` 提交终态。

## 重新评估条件

当 Task 状态与事件需要发布到外部消息系统时，在现有数据库事务中增加 Outbox，而不是直接双写外部系统。
