# ADR-0004：原子提交任务领取与领取事件

- 状态：已采纳
- 日期：2026-07-28

## 背景

Worker 原先先调用 `TaskStore.ClaimNext` 将任务改为 `running`，再单独写入
`task_started` 或 `task_recovered` 事件。如果进程在两个动作之间崩溃，或者事件写入失败，
系统会出现“任务已经运行，但没有领取事件”的不一致状态。

## 决策

通过 `TaskTransitionStore.ClaimNextWithEvent` 在同一原子边界中完成：

```text
BEGIN
→ 选择 queued 任务或租约过期的 running 任务
→ 更新状态、租约、重试次数和版本号
→ 写入 task_started 或 task_recovered 事件
→ COMMIT
```

MySQL 实现使用同一个数据库事务。内存实现同时持有 Task Store 和 Event Store 的写锁，
并在事件构造失败时恢复领取前的任务快照。

事件 ID 由 Worker 生成；事件类型和消息由 Domain 根据领取后的 `RetryCount` 决定：
首次领取产生 `task_started`，崩溃恢复后的再次领取产生 `task_recovered`。

## 原因

- 任务状态与生命周期事件表达的是同一个业务事实。
- 事务失败时不应留下只有一半成立的状态。
- 事件语义属于领域规则，不应硬编码在 MySQL Store 中。
- Worker 只编排用例，不负责拼接多个无法保证原子的存储操作。

## 代价与边界

- `TaskTransitionStore` 同时协调 Task 与 Event，接口比单表 Store 更高层。
- 内存实现必须保持固定加锁顺序，避免与其他跨 Store 操作形成死锁。
- 当前只保证领取事件与领取原子；Reaper 的失败清理与失败事件仍需单独改造。
- 本决策不保证 Executor 只执行一次。租约恢复可能造成至少一次执行语义，副作用任务仍需幂等。
