# ADR 0004：使用 Redis Streams 承载高吞吐任务分发

## 状态

提议，尚未实施。

## 背景

MemoBridge 当前可以为单份 SourceItem 创建一个
`memobridge.semantic_profile` 任务。这个场景在少量资料下使用 MySQL
轮询和 `ClaimNext` 已经足够。

当用户一次导入几千甚至几万份资料时，系统会出现新的压力：

- 大量 Worker 周期性扫描 MySQL 的 queued 任务；
- 没有新任务时仍然产生无效查询；
- 多个 Worker 同时竞争任务行锁；
- 任务已经写入数据库，但 Worker 要等下一次轮询才能发现；
- 任务分发吞吐受数据库查询和锁竞争限制。

因此 Redis 的业务场景不是单个 LLM 请求，而是：

> MemoBridge 批量导入大量资料后，TaskPulse 需要把大量独立分析任务快速分发给多个 Worker。

## 决策

引入 Redis Streams 作为“待执行任务的分发通道”，但不改变任务状态的所有权：

```text
MySQL
  保存 Task、状态、租约、重试次数和事件

Redis Streams
  保存待分发的 task_id 通知

TaskPulse
  从 Redis 获取 task_id
  再通过 MySQL 原子 Claim 建立 lease

MemoBridge Worker
  仍然只调用 TaskPulse Worker API
```

Redis 中只保存 `task_id`、workflow 和投递时间等最小元数据，不能保存
SourceItem 正文、Prompt 或完整 LLM 输出。

## 为什么不能只使用 Redis

Redis 不是任务最终事实来源，原因包括：

- 任务结果和状态需要长期持久化；
- 重试、租约恢复和审计事件已经由 MySQL 模型定义；
- Redis 消息可能重复投递；
- Redis 重启或网络故障不能导致任务状态丢失。

所以 Redis 只负责“尽快告诉 Worker 有任务”，MySQL 负责回答“任务当前到底是什么状态”。

## 可靠投递方案

任务创建和 Redis 投递不能直接依赖两个独立操作，否则可能出现：

```text
MySQL 创建成功，但 Redis 发布失败
```

第一版采用 Outbox：

1. MySQL 事务同时写入 Task 和 `task_outbox`；
2. Publisher 扫描未发布的 outbox 记录；
3. Publisher 使用 `XADD` 写入 Redis Stream；
4. 发布成功后把 outbox 标记为 published；
5. Publisher 崩溃时允许重复发布；
6. TaskPulse 的 MySQL Claim 保证重复通知不会产生两个有效 lease。

如果 Redis 不可用，TaskPulse 保留 MySQL 定期扫描作为降级路径，确保任务最终仍可执行。

## 不变的并发保证

Redis Consumer Group 只能减少重复读取，不能替代 MySQL 的并发控制。

真正的并发保证仍然是：

```text
Redis 读取 task_id
  -> MySQL 条件更新 queued -> running
  -> 成功者获得 lease_token
  -> 其他 Worker 领取失败
```

因此 Redis 消息重复、Worker 重启或 Publisher 重试都不会破坏任务状态机。

## 验收场景

使用批量 SemanticProfile 生成作为真实负载：

1. 创建 10,000 个资料分析任务；
2. 比较 MySQL 轮询模式和 Redis Streams 模式的任务发现延迟；
3. 比较 MySQL 查询次数、锁等待和吞吐；
4. Redis 暂停期间确认任务不会丢失；
5. Publisher 重复发布时确认不会产生重复执行；
6. Worker 崩溃后确认仍由 MySQL 租约恢复；
7. Redis 恢复后确认积压任务可以继续分发。

## 暂不做

- 不使用 Kafka 替代 Redis；
- 不使用 Redis 保存任务最终状态；
- 不把 Redis Streams 直接暴露给 MemoBridge；
- 不删除现有 MySQL Claim 和 Lease；
- 没有基准数据前不宣称 Redis 一定更快。

## 结果

这个演进让 Redis 有明确的工程动机：

```text
批量资料导入
  -> 任务数量变大
  -> MySQL 轮询和锁竞争成为瓶颈
  -> Redis Streams 提高任务发现和分发效率
  -> MySQL 继续保证状态、租约和恢复的一致性
```
