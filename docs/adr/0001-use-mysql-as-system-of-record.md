# ADR-0001：使用 MySQL 8 作为任务状态的持久化真相源

- 状态：已接受，待实现
- 日期：2026-07-23
- 决策者：TaskPulse 项目维护者

## 背景

TaskPulse 当前使用 `MemoryTaskStore` 和 `MemoryEventStore`。这让我们可以先验证任务状态机、应用层、Worker 和执行器边界，但它存在明确限制：

1. 进程退出后任务和事件全部丢失。
2. `sync.RWMutex` 只能协调同一进程内的 goroutine，不能协调多个 API 或 Worker 实例。
3. Task 与 `task_created` Event 分两次写入，无法保证原子性。
4. Worker 领取状态无法跨进程共享，也无法支持崩溃后的恢复。
5. 无法通过唯一约束实现持久化幂等。

可靠异步任务系统必须先解决状态持久化和并发协调，之后讨论 Redis 队列、Kubernetes 多副本才有意义。

## 决策

TaskPulse 下一阶段使用 **MySQL 8.4 LTS + InnoDB**。本地开发镜像固定为 `mysql:8.4.10`：

- MySQL 是 Task、TaskEvent 和执行结果的持久化真相源。
- 第一版先使用 MySQL 表作为持久化任务队列。
- 创建 Task 和 Created Event 必须放在同一数据库事务中。
- Worker 在事务中使用锁定读领取任务，并通过条件更新完成状态变更。
- 后续增加 `lease_owner`、`lease_expires_at` 支持 Worker 崩溃恢复。
- 后续增加 `idempotency_key` 唯一索引支持重复请求去重。

计划中的领取过程：

```text
开始事务
→ SELECT queued task ... FOR UPDATE SKIP LOCKED
→ UPDATE task SET status='running', lease_owner=?, lease_expires_at=?
→ 写入 task_started 事件
→ 提交事务
```

`SKIP LOCKED` 让多个 Worker 跳过已经被其他事务锁定的任务，减少领取阶段的相互等待。但它不是完整的可靠性方案；租约、幂等、重试和事务边界仍需单独设计。

## 为什么选择 MySQL

### 1. 数据形态适合关系模型

TaskPulse 的核心数据具有明确关系和约束：

```text
Task 1 → N TaskEvent
Task.id 唯一
IdempotencyKey 唯一
状态、租约和重试字段需要条件查询与索引
```

关系数据库能直接表达主键、唯一约束、索引和事务。

### 2. 同时解决持久化和并发协调

InnoDB 提供事务、崩溃恢复和行级锁。第一版不需要为了任务分发立即增加第二个基础设施组件，能够先把一致性问题集中在一个系统内。

### 3. 与项目学习目标匹配

这个选择会自然产生可验证的问题：事务隔离、索引设计、锁竞争、死锁重试、条件更新和连接池。它们都是 TaskPulse 当前可靠性目标需要的能力，而不是为了简历堆叠技术。

### 4. 与目标岗位具有现实相关性

目标岗位明确涉及 MySQL/TiDB/CockroachDB。岗位匹配是次要收益，不是主要技术理由；即使没有招聘要求，TaskPulse 仍然需要一个支持事务与并发控制的持久化真相源。

## 备选方案

### 继续使用内存 Store

拒绝。它适合单元测试和原型，但不能满足重启恢复、多实例协调和持久化幂等。

### PostgreSQL

技术上完全可行，也支持事务、行锁和 `SKIP LOCKED`。项目没有依赖 PostgreSQL 独有能力；在功能近似的情况下，选择与个人已有知识和目标岗位更匹配的 MySQL，减少一个月周期内的学习分叉。

### 只使用 Redis

暂不选择。Redis 很适合队列、限流和短期状态，但 TaskPulse 仍需要长期任务记录、事务约束和查询能力。直接让 Redis 同时承担真相源与队列，会把持久化、一致性和查询问题混在一起。

### Redis Streams + MySQL

暂缓。它能提供消费组、确认和 Pending 消息恢复，但会立即引入 MySQL/Redis 双写一致性，需要 Outbox Relay。先完成 MySQL 基线和压力测试，再通过单独 ADR 决定是否引入。

### Kafka

拒绝当前引入。当前没有长周期事件保留、多消费组回放或足以证明 Kafka 运维成本合理的吞吐需求。

## 代价与风险

- 需要管理 Schema、迁移、连接池和集成测试环境。
- 锁定读可能产生锁等待或死锁，必须保持事务短小并设计合适索引。
- 数据库轮询会增加查询压力，需要通过指标和压测确定边界。
- `SKIP LOCKED` 提供领取并发，不提供 exactly-once；Worker 仍可能在完成副作用后、提交状态前崩溃。
- MySQL 成为关键依赖，需要健康检查、超时和故障处理。

## 验证标准

本决策只有通过以下实验才算落地：

1. 创建任务后重启 TaskPulse，任务和事件仍可查询。
2. 并发启动多个 Worker，同一 queued 任务只有一个 Worker 成功领取。
3. Task 与 Created Event 任一写入失败时，事务整体回滚。
4. 重复 `idempotency_key` 只产生一个任务。
5. Worker 领取后被杀死，租约过期后任务可以再次领取。
6. 记录不同并发 Worker 数下的领取吞吐、锁等待和 P95 延迟。

## 重新评估条件

出现以下证据时，重新评估是否增加 Redis Streams：

- MySQL 轮询或锁竞争成为已测量的主要瓶颈。
- 需要更低延迟的任务唤醒和独立消费者组。
- 已经设计并验证 MySQL Outbox，能够处理双写一致性。

## 参考资料

- [MySQL 8.4 参考手册](https://dev.mysql.com/doc/refman/8.4/en/)
- [MySQL 8.4 锁定读与 SKIP LOCKED](https://dev.mysql.com/doc/refman/8.4/en/innodb-locking-reads.html#innodb-locking-reads-nowait-skip-locked)
- [MySQL 8.4 InnoDB 锁](https://dev.mysql.com/doc/refman/8.4/en/innodb-locks-set.html)
