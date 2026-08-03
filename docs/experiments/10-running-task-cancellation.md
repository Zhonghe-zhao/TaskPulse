# 10：运行中任务取消实验

## 目的

验证任务已经进入 `running` 后，调用取消接口可以停止 Executor，并且旧 Worker 不能再提交成功结果。

## 环境

- 存储：MySQL
- Workflow：`llm_analysis`
- Fake LLM 延迟：`10s`
- Worker 租约：建议 `3s`（续租周期约 1s，便于在 Fake 延迟窗口内观察到租约失效）；使用默认 30s 也可，但需等待更久

## 步骤

1. 启动 TaskPulse：

```powershell
$env:TASKPULSE_STORAGE="mysql"
$env:TASKPULSE_HTTP_ADDR=":8080"
$env:TASKPULSE_WORKER_COUNT="1"
$env:TASKPULSE_LLM_FAKE_DELAY="10s"
$env:TASKPULSE_WORKER_LEASE_DURATION="3s"
go run ./cmd/taskpulse
```

2. 创建一个 `llm_analysis` 任务。

3. 查询任务，确认状态为 `running`。

4. 调用取消接口：

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri "http://localhost:8080/tasks/{task_id}/cancel"
```

5. 再次查询任务和事件。

期望任务状态为：

```text
canceled
```

事件中应包含：

```text
task_created
task_started
task_canceled
```

## 关键保证

- 取消请求在事务中清除租约并增加任务版本。
- Worker 下一次续租时发现租约条件不再满足，取消 Executor Context。
- Worker 不会将取消任务提交为 succeeded、partially_succeeded 或 failed。
- 如果任务已经先完成，取消请求会看到终态并返回不可取消错误；不会覆盖已经提交的结果。

## 结论

运行中取消依赖的是数据库状态、租约条件和版本冲突，而不是进程内的共享变量。因此即使 API 和 Worker 位于不同进程，取消仍然可以生效。

## 实际结果

环境：MySQL；单进程 Worker=1；`TASKPULSE_LLM_FAKE_DELAY=10s`；`TASKPULSE_WORKER_LEASE_DURATION=3s`（缩短租约以便快速观测续租失败）；HTTP `:8082`。

步骤与观察：

1. 创建 `llm_analysis` 任务后很快进入 `running`。
2. 调用 `POST /tasks/{id}/cancel`，响应立即返回 `status=canceled`，`version=2`。
3. 最终查询任务状态仍为 `canceled`，`finished_at` 已写入。
4. 事件序列为：

```text
task_created → task_started → task_canceled
```

5. 事件中**没有** `task_succeeded` / `task_failed`。
6. 示例任务 ID：`task_1785748820698723400_116`。

因此，本实验验证了 running 任务取消后状态与事件正确收敛，旧 Worker 未覆盖终态。

## 当前边界

- 取消是协作式的，Executor 必须正确响应 Context。
- 不响应 Context 的外部调用无法被强制杀死，只能等待租约恢复机制处理。
- 取消接口当前没有单独的取消原因字段。
