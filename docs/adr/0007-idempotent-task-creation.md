# ADR-0007：使用 Idempotency-Key 保证任务创建幂等

- 状态：已接受，已实现，待验证
- 日期：2026-07-29

## 背景

上层业务调用 `POST /tasks` 时，TaskPulse 可能已经创建任务，但响应因为网络中断或超时
没有到达调用方。调用方重试请求后，如果每次都创建新任务，同一业务操作会被执行多次。
对于 LLM、Skill 和其他有外部副作用或费用的任务，这会造成重复结果和重复成本。

MySQL Schema 已有可空的 `idempotency_key` 唯一列，但 Domain、Application、HTTP 和
Store 尚未使用它。

## 决策

客户端可以通过 HTTP Header 提供可选幂等键：

```http
POST /tasks
Idempotency-Key: memobridge-analysis-123
```

语义固定为：

```text
无 Key                    → 每次创建新任务
相同 Key + 相同创建参数    → 返回第一次创建的任务
相同 Key + 不同创建参数    → 409 Conflict
```

用于判断“相同创建参数”的字段只有：

- `workflow`
- `input` 的 JSON 语义值
- `max_retries`

任务 ID、事件 ID、创建时间和状态不参与比较。JSON 比较忽略无意义的空白和对象字段顺序。

幂等键是大小写敏感的 opaque string，长度为 1 到 128 字节，不允许首尾空白。
MySQL 使用 `VARBINARY(128)`，避免表级大小写不敏感排序规则改变该语义。

首次创建返回 `201 Created`；幂等重放返回 `200 OK`。无论重放多少次，都只能存在一个
Task 和一个 `task_created` 事件。

## 并发语义

Memory 实现使用 Task Store 和 Event Store 的同一组写锁，并维护
`idempotency_key → task_id` 索引。

MySQL 实现依赖 `uk_tasks_idempotency_key` 唯一约束处理并发竞争：

```text
事务 A INSERT 成功并提交
事务 B INSERT 等待后命中唯一键
事务 B 查询已有任务
→ 参数相同：返回已有任务
→ 参数不同：返回冲突
```

不能使用“先 SELECT、没有再 INSERT”作为唯一保证，因为两个并发事务可能同时看不到记录。
数据库唯一约束是最终并发裁决者。

## 备选方案

### 由 MemoBridge 自己避免重复请求

不采用。调用方无法判断响应丢失时 TaskPulse 是否已经提交，幂等性必须由接收请求的一方保证。

### 根据 input 内容自动生成幂等键

不采用。相同输入可能是用户有意创建的两个任务，是否属于同一个业务操作应由调用方决定。

### Redis SETNX

暂不采用。MySQL 已经是任务真相源并具备唯一约束，引入 Redis 会产生额外一致性问题。

## 代价与风险

- Task 模型和所有 MySQL SELECT/INSERT 映射需要增加 `idempotency_key`。
- 创建 Store 需要返回“新建还是重放”，HTTP 才能选择 201 或 200。
- 幂等键长期保留会阻止相同 Key 再次创建；第一版不提供过期和删除语义。
- 该能力只防止重复创建任务，不保证 Executor 的外部副作用幂等。

## 验证标准

- 无 Key 的两次请求创建两个不同任务。
- 相同 Key、相同参数返回同一个任务。
- 相同 Key、不同参数返回 `ErrIdempotencyConflict` 和 HTTP 409。
- 并发提交相同 Key 时只有一个 Task 和一个 `task_created` 事件。
- 服务重启后，MySQL 仍能识别幂等重放。

## 重新评估条件

- 需要按租户隔离相同幂等键；
- 需要为幂等键设置有效期；
- 创建参数扩大到优先级、超时或调用方身份；
- 需要对 Executor 副作用提供独立幂等保证。
