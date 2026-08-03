# 04：可观测性基线

## 问题

可靠任务系统不能只回答“任务是否完成”，还需要回答：

```text
任务有没有被 Worker 领取？
任务失败在哪里？
哪些错误触发了重试？
任务执行耗时是多少？
Worker 是否丢失租约？
Reaper 是否清理了过期任务？
```

如果没有日志和指标，任务系统出问题时只能靠查询数据库或猜测。

## 假设

第一版可观测性不需要先引入完整 Prometheus client 和 Grafana。

使用标准库 `log/slog` 输出结构化文本日志，并用 `/metrics` 暴露 Prometheus 文本格式，
已经足够支撑本阶段的调试、演示和后续压测。

## 环境

- 应用：TaskPulse 单进程 API + Worker + Reaper。
- 日志：`slog.TextHandler` 输出到 stdout。
- 指标：`GET /metrics`。
- 第三方依赖：无新增依赖。

## 步骤

1. 启动 TaskPulse。

```powershell
$env:TASKPULSE_STORAGE="memory"
go run ./cmd/taskpulse
```

2. 提交一个 `llm_analysis` 任务。

3. 观察 stdout 日志。

期望出现：

```text
msg="task claimed" task_id=... workflow=llm_analysis worker_id=...
msg="task completed" task_id=... workflow=llm_analysis status=succeeded duration_ms=...
```

4. 查询 `/metrics`。

```powershell
Invoke-RestMethod -Method Get -Uri http://localhost:8080/metrics
```

期望包含：

```text
taskpulse_tasks_claimed_total{workflow="llm_analysis"} 1
taskpulse_tasks_completed_total{workflow="llm_analysis",status="succeeded"} 1
taskpulse_task_execution_duration_seconds_count{workflow="llm_analysis"} 1
taskpulse_tasks_current{status="succeeded"} 1
```

5. 可选：开启一次限流重试。

```powershell
$env:TASKPULSE_LLM_FAKE_FAILURE="rate_limited_once"
```

重新提交任务后，期望日志出现 `task scheduled for retry`，指标出现：

```text
taskpulse_tasks_retried_total{workflow="llm_analysis",error_code="llm_rate_limited"} 1
```

## 指标

当前暴露：

- `taskpulse_tasks_claimed_total{workflow}`
- `taskpulse_tasks_completed_total{workflow,status}`
- `taskpulse_tasks_retried_total{workflow,error_code}`
- `taskpulse_tasks_current{status}`
- `taskpulse_tasks_available_current{status}`
- `taskpulse_oldest_available_task_age_seconds{status}`
- `taskpulse_lease_renewed_total{workflow}`
- `taskpulse_lease_lost_total{workflow}`
- `taskpulse_reaper_expired_failures_total{workflow}`
- `taskpulse_task_execution_duration_seconds`

## 结论

这一步把 TaskPulse 从“能执行任务”推进到“能观察任务执行”：

```text
日志解释单个任务发生了什么
计数器解释 Worker 做过什么
队列指标解释当前还有多少任务等待处理
```

日志级别按事件风险区分：正常成功使用 `INFO`，部分成功和可恢复重试使用 `WARN`，
最终失败使用 `ERROR`，高频续租成功使用 `DEBUG`。这样终端默认不会把所有事件混在同一等级中。

它为后续压测和故障实验提供基础证据。

## 边界

当前没有实现：

- Grafana Dashboard；
- Prometheus 官方 Go client；
- pprof；
- k6 压测报告。

这些可以在后续工程证据阶段继续补充。
