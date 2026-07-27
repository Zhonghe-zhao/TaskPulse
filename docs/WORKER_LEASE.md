# Worker 租约设计

## 要解决的问题

任务被 Worker 领取并变成 `running` 后，Worker 可能在写回结果前退出。如果系统只记录任务状态，就无法判断这个任务仍在执行，还是已经失去执行者。

租约为一次领取增加两个事实：

- `lease_owner`：当前领取任务的 Worker；
- `lease_expires_at`：本次执行权的失效时间。

租约表示的是一段有限时间内的执行权，不表示任务一定执行成功。

## 当前实现

Worker 调用：

```text
ClaimNext(worker_id, now, lease_duration)
```

MySQL 在同一事务中完成：

```text
锁定最早的 queued 任务
→ 跳过其他事务已锁定的任务
→ 修改为 running
→ 写入租约所有者和过期时间
→ version + 1
→ 提交事务
```

当前保证：

- 多个 Worker 不会同时成功领取同一条 `queued` 任务；
- 领取结果可以追溯到具体 Worker；
- 任务进入终态时清除租约；
- 旧版本 Worker 写回结果时会受到乐观锁保护。
- Worker 执行任务期间按租约时长的三分之一周期续租；
- 只有租约未过期且 `lease_owner` 匹配的 Worker 可以续租；
- 续租不修改任务业务版本，避免心跳与完成写回发生版本冲突。
- 过期的 `running` 任务可以被新 Worker 原子接管；
- 接管会增加 `retry_count` 和 `version`，隔离旧 Worker 的写回；
- 接管产生 `task_recovered` 事件。
- Reaper 将重试额度耗尽的过期任务转换为 `failed`；
- 清理失败任务时会清除租约、记录结束时间、增加版本并写入失败事件。

## 尚未实现

- Worker 崩溃恢复实验；
- 重复执行时的业务幂等。
- 可查询和人工重放的死信任务。

只有 `retry_count < max_retries` 的过期任务可以被接管。达到重试上限后，Reaper 会将任务收敛为 `failed`，避免任务永久停留在 `running`。

## 交付语义

未来加入过期恢复后，TaskPulse 提供的是 `at-least-once`（至少一次）执行，而不是 `exactly-once`（恰好一次）执行。

原因是 Worker 可能已经完成外部副作用，但在写回任务结果前崩溃。系统无法仅通过任务表判断副作用是否发生，因此恢复后可能再次执行。最终需要执行器使用幂等键、唯一约束或结果去重来处理重复执行。

## 下一步

1. 编写停止 Worker 进程后的恢复实验。
2. 为执行器补充幂等约束。
3. 设计死信查询和人工重放接口。
