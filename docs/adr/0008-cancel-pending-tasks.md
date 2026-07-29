# ADR-0008：原子取消尚未开始执行的任务

- 状态：已接受，已实现，待验证
- 日期：2026-07-29

## 背景

上层业务创建异步任务后，可能因为用户撤销操作、输入已经失效或业务流程结束而不再需要执行。
如果任务仍在排队或等待重试，TaskPulse 应阻止 Worker 后续领取它。

“取消尚未执行的任务”和“中断正在运行的任务”并不是同一个问题：

- 前者只需要在领取竞争中原子改变持久化状态；
- 后者还需要运行中的 Worker 感知请求，并通过 `context.Context` 通知 Executor 停止。

如果 API 直接把 `running` 改成 `canceled`，Executor 仍可能继续调用 LLM 或外部 Skill，
系统会出现“状态显示取消，但副作用仍在发生”的错误保证。

## 决策

第一版提供：

```http
POST /tasks/{task_id}/cancel
```

状态语义：

```text
queued   → canceled + task_canceled event
retrying → canceled + task_canceled event
canceled → 返回已有任务，不重复写事件
running  → 409 Conflict
其他终态 → 409 Conflict
不存在   → 404 Not Found
```

首次取消和重复取消都返回 `200 OK`。响应中的 Task 是取消后的持久化状态。

取消状态与 `task_canceled` 事件必须在同一个 Memory 临界区或 MySQL 事务中提交。
MySQL 使用 `SELECT ... FOR UPDATE` 锁定目标行，因此取消和 Worker 领取不能同时成功：

```text
取消先获得锁 → 状态变成 canceled → Worker 无法领取
领取先获得锁 → 状态变成 running  → 取消返回 409
```

`canceled` 是终态；取消后清除租约并设置 `finished_at`。取消不增加重试次数，也不删除任务历史。

## 为什么不直接复用 Get + UpdateTaskWithEvent

应用层先查询再更新会在两次数据库操作之间留下竞争窗口。版本号可以发现冲突，但无法直接提供
稳定的幂等返回语义。取消 Store 在单个事务中完成读取、状态判断、更新和事件写入。

## 暂不实现运行中取消

运行中任务取消需要新增独立协议：

- 持久化 `cancel_requested_at` 或等价字段；
- Worker 周期性观察取消请求；
- Worker 持有当前 Executor 的 `context.CancelFunc`；
- Executor 必须正确响应 `ctx.Done()`；
- 明确外部调用已经发出时的副作用语义；
- Worker 崩溃、租约过期和取消请求之间仍需确定优先级。

在这些能力完成前，TaskPulse 不宣称支持停止运行中的任务。

## 验证标准

- queued 和 retrying 任务可以取消，并各自只产生一个取消事件。
- 重复取消返回相同任务，不产生第二个取消事件。
- running、succeeded、partially_succeeded 和 failed 任务返回不可取消冲突。
- 不存在的任务返回 Not Found。
- MySQL 中状态和事件能够一起提交或一起回滚。
- 并发领取与取消时，只能有一个状态转换成功。

## 重新评估条件

- Agent/LLM Executor 出现分钟级任务，需要用户主动中断；
- 需要区分用户取消、系统取消和超时取消；
- 需要批量取消、按业务主体取消或级联取消子任务；
- 外部工具调用需要补偿或副作用幂等协议。
