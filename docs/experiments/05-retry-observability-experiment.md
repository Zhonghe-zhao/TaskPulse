# 05：LLM 限流重试可观测性实验

## 问题

TaskPulse 已经支持 transient error、`retrying` 状态、`available_at` 延迟领取和
`task_retrying/task_retry_started` 事件。

但基础设施项目不能只说“支持重试”，还需要能展示：

```text
任务为什么进入重试？
重试前后状态如何变化？
哪些指标能看到这次重试？
任务最终是否成功？
```

## 假设

使用 `llm_analysis` 的 Fake Client 模拟一次模型限流，可以稳定复现：

```text
第一次执行返回 llm_rate_limited
→ Worker 将任务转入 retrying
→ 到 available_at 后再次领取
→ 第二次执行成功
```

这个实验验证的是 TaskPulse 的通用重试和观测链路，不验证真实 LLM Provider。

## 环境

- 存储：建议先用 `memory`，便于重复实验。
- Workflow：`llm_analysis`。
- Fake Provider 故障模式：`rate_limited_once`。
- HTTP 服务端口：`:8080`。

## 步骤

1. 启动 TaskPulse。

```powershell
$env:TASKPULSE_STORAGE="memory"
$env:TASKPULSE_LLM_FAKE_FAILURE="rate_limited_once"
go run ./cmd/taskpulse
```

2. 创建 `llm_analysis` 任务。

```powershell
$body = @{
  workflow = "llm_analysis"
  input = @{
    subject = "Go concurrency"
    notes = @("goroutine", "channel", "context")
    goal = "make a two week study plan"
  }
  max_retries = 3
} | ConvertTo-Json -Depth 5

$task = Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8080/tasks `
  -ContentType "application/json" `
  -Body $body
```

3. 立刻查询任务事件。

```powershell
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/tasks/$($task.id)/events"
```

可能看到：

```text
task_created
task_started
task_retrying
```

4. 查询 metrics。

```powershell
Invoke-RestMethod -Method Get -Uri http://localhost:8080/metrics
```

重试等待期间重点观察：

```text
taskpulse_tasks_retried_total{workflow="llm_analysis",error_code="llm_rate_limited"} 1
taskpulse_tasks_current{status="retrying"} 1
```

如果 `available_at` 已经到期但 Worker 尚未再次领取，可能看到：

```text
taskpulse_tasks_available_current{status="retrying"} 1
taskpulse_oldest_available_task_age_seconds{status="retrying"} ...
```

5. 等待 1 到 2 秒后再次查询任务。

```powershell
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/tasks/$($task.id)"
```

期望最终状态：

```text
succeeded
```

6. 再次查询事件。

```powershell
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/tasks/$($task.id)/events"
```

期望完整事件链：

```text
task_created
task_started
task_retrying
task_retry_started
task_succeeded
```

7. 再次查询 metrics。

最终应包含：

```text
taskpulse_tasks_claimed_total{workflow="llm_analysis"} 2
taskpulse_tasks_retried_total{workflow="llm_analysis",error_code="llm_rate_limited"} 1
taskpulse_tasks_completed_total{workflow="llm_analysis",status="succeeded"} 1
taskpulse_tasks_current{status="succeeded"} 1
taskpulse_task_execution_duration_seconds_count{workflow="llm_analysis"} 1
```

## 指标解释

- `taskpulse_tasks_claimed_total=2`：第一次执行和重试执行各领取一次。
- `taskpulse_tasks_retried_total=1`：只有第一次限流触发重试。
- `taskpulse_tasks_completed_total=1`：最终只产生一个终态成功。
- `taskpulse_tasks_current{status="retrying"}`：重试等待窗口内可观察。
- `taskpulse_oldest_available_task_age_seconds`：可用于发现任务到期后仍长期未被消费。

## 结论

这个实验把“错误分类 + 延迟重试 + 再次领取 + 最终成功 + 指标观测”串成一条证据链。

面试时可以这样讲：

> 我没有简单地失败后立即重试，而是把临时错误转为 retrying 状态，记录 error_code 和
> available_at。Worker 只会领取到期任务，事件链能还原重试过程，metrics 能看到重试次数、
> 当前 retrying 数量和最老可领取任务等待时间。

## 边界

当前实验没有验证：

- 真实 LLM Provider 的 429/5xx/timeout 映射；
- 多 Worker 下的重试竞争；
- MySQL 持久化重启后的 retrying 恢复；
- 任务长时间运行时的 lease renewal 指标。

这些应在后续故障恢复和多 Worker 实验中验证。
