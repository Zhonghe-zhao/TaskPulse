# 07：多 Worker 并发领取实验

## 目的

验证多个任务级 Worker 可以并发处理任务，并且竞争同一个任务时不会同时有效领取它。

## 环境

- 存储：MySQL
- Worker 数量：4
- Workflow：`llm_analysis`
- Fake LLM 延迟：`3s`

## 步骤

1. 启动 TaskPulse：

```powershell
$env:TASKPULSE_STORAGE="mysql"
$env:TASKPULSE_WORKER_COUNT="4"
$env:TASKPULSE_LLM_FAKE_DELAY="3s"
go run ./cmd/taskpulse
```

启动日志应出现：

```text
TaskPulse worker count: 4
```

2. 连续创建至少 8 个 `llm_analysis` 任务。

3. 观察日志中的 `worker_id`。任务应由多个不同 Worker 并发领取，而不是全部由同一个 Worker 顺序处理。

4. 查询每个任务的事件。每个任务都应只有一个初始 `task_started`，最终进入 `task_succeeded`。

5. 查询 `/metrics`，确认 `taskpulse_tasks_claimed_total` 反映所有 Worker 的领取总数。

## 观察点

- 多个 Worker 具有不同的 `worker_id`。
- 任务可以被多个 Worker 并发处理。
- 同一个任务不会被两个 Worker 同时成功领取。
- Worker 数量增加后，整体完成时间应低于单 Worker 情况，但不会无限线性下降。

## 结论

TaskPulse 的并发不是简单地启动大量 goroutine，而是让多个独立 Worker 通过持久化任务队列竞争工作。MySQL 事务、行锁和 `SKIP LOCKED` 共同保证任务领取的互斥性。

未来拆分成独立 Worker 进程时，仍然可以复用同一套领取语义。

## 当前边界

- 本实验只验证同一进程内的多个 Worker。
- 尚未比较不同 Worker 数量下的 P50/P95/P99 延迟。
- 尚未验证多个 TaskPulse 进程之间的竞争。
