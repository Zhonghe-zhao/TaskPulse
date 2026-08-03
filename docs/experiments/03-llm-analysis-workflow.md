# 03：LLM Analysis Workflow 执行闭环

## 问题

TaskPulse 已经通过 `url_check` 验证了网络 IO 任务，但如果系统只支持一种 URL 检测
Executor，容易被误解为专用脚本，而不是通用异步任务执行基础设施。

需要验证：

```text
同一套 Task、Store、Worker、状态机、事件和重试模型
能否承载第二种任务类型：LLM Analysis
```

## 假设

只要新的任务类型实现 `worker.Executor` 接口，并在启动组装层注册到 workflow registry，
Worker 就不需要理解具体业务，也能完成：

```text
claim task
→ execute workflow
→ persist result
→ append terminal event
```

## 环境

- 存储：`memory` 或 `mysql` 均可。
- Workflow：`llm_analysis`。
- Provider：Fake LLM Client。
- 真实模型：未接入。

## 步骤

1. 启动 TaskPulse。

```powershell
$env:TASKPULSE_STORAGE="memory"
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

Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8080/tasks `
  -ContentType "application/json" `
  -Body $body
```

3. 使用返回的 `id` 查询任务。

```powershell
Invoke-RestMethod -Method Get -Uri http://localhost:8080/tasks/{task_id}
```

4. 查询任务事件。

```powershell
Invoke-RestMethod -Method Get -Uri http://localhost:8080/tasks/{task_id}/events
```

5. 可选：模拟一次 LLM 限流后成功。

重新启动服务前设置：

```powershell
$env:TASKPULSE_LLM_FAKE_FAILURE="rate_limited_once"
```

再次创建同样的 `llm_analysis` 任务，观察任务是否先进入 `retrying`，随后再次被领取并成功。

## 指标

本实验先观察功能闭环，不做压测指标。

记录：

- 任务最终状态；
- `result` 是否包含结构化输出；
- 事件序列是否完整；
- 模拟限流时是否产生 `task_retrying` 和 `task_retry_started`；
- Worker 是否需要理解 LLM 业务细节。

## 期望结果

任务最终状态：

```text
succeeded
```

任务结果应包含：

```json
{
  "subject": "Go concurrency",
  "summary": "...",
  "plan": ["..."],
  "model": "fake-llm"
}
```

事件序列：

```text
task_created
task_started
task_succeeded
```

模拟一次限流时，事件序列应包含：

```text
task_created
task_started
task_retrying
task_retry_started
task_succeeded
```

## 结论

`llm_analysis` 证明 TaskPulse 的核心 Worker 链路不依赖 URL 业务：

```text
workflow registry 选择 Executor
Executor 返回 ExecutionResult
Worker 统一处理状态、结果和事件
```

这说明 TaskPulse 的技术重点不是某个具体业务，而是可靠执行模型。

## 边界

当前实验不证明：

- 已经接入真实 LLM Provider；
- 已经记录 token 用量；
- 已经支持流式输出；
- 已经支持多 Agent 协作；
- 已经支持 MemoBridge 真实 SourceItem 接入。

这些能力需要后续单独设计、实现和验证。
