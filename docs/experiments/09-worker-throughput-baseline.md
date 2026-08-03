# 09：Worker 吞吐基线实验

## 目的

测量任务级 Worker 数量对吞吐和队列等待时间的影响，回答：

```text
增加 Worker 是否提高吞吐？
吞吐是否接近线性增长？
什么时候出现数据库或调度瓶颈？
```

## 控制变量

- 存储：MySQL
- Workflow：`llm_analysis`
- Fake LLM 延迟：`1s`
- 任务数量：40
- Worker 数量：1、2、4
- 每次实验使用新的任务，避免复用历史终态任务

## 启动配置

每次只修改 Worker 数量：

```powershell
$env:TASKPULSE_STORAGE="mysql"
$env:TASKPULSE_HTTP_ADDR=":8080"
$env:TASKPULSE_WORKER_COUNT="1"
$env:TASKPULSE_LLM_FAKE_DELAY="1s"
go run ./cmd/taskpulse
```

分别将 `TASKPULSE_WORKER_COUNT` 改为 `2` 和 `4`，重复实验。

## 任务提交

提交 40 个输入相同但任务 ID 不同的 `llm_analysis` 任务。可以使用 PowerShell 循环调用现有 `POST /tasks` 接口；每次请求都记录返回的任务 ID。

记录两个时间点：

```text
T_submit：提交第一个任务前的时间
T_done：40 个任务全部变成 succeeded 后的时间
```

总完成时间：

```text
duration = T_done - T_submit
```

吞吐：

```text
throughput = 40 / duration_seconds
```

## 观察指标

在任务执行期间和结束后查询：

```powershell
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/metrics"
```

重点记录：

- `taskpulse_tasks_claimed_total`
- `taskpulse_tasks_completed_total`
- `taskpulse_tasks_current{status="running"}`
- `taskpulse_tasks_available_current{status="queued"}`
- `taskpulse_oldest_available_task_age_seconds{status="queued"}`
- `taskpulse_task_execution_duration_seconds`

## 结果表

墙钟时间：`T_done - T_submit`。单任务延迟由字段计算：

```text
queue_wait = started_at - created_at
execution  = finished_at - started_at
total      = finished_at - created_at
```

分位数采用 nearest-rank（对已排序样本取 `ceil(p/100*n)` 位置），样本为各轮 40 个 succeeded 任务。

### 批次吞吐

| Worker 数量 | 任务数量 | Fake 延迟 | 总耗时 | 吞吐 | 结论 |
|---:|---:|---:|---:|---:|---|
| 1 | 40 | 1s | 41.131s | 0.973/s | 串行执行，排队最长 |
| 2 | 40 | 1s | 20.688s | 1.934/s | 吞吐约 1.99× |
| 4 | 40 | 1s | 11.117s | 3.598/s | 吞吐约 3.70× |

### 队列等待 `queue_wait`（秒）

| Worker | P50 | P95 | P99 | avg | max |
|---:|---:|---:|---:|---:|---:|
| 1 | 19.450 | 37.731 | 39.758 | 19.969 | 39.758 |
| 2 | 9.252 | 18.321 | 19.327 | 9.669 | 19.327 |
| 4 | 4.703 | 9.270 | 9.671 | 4.849 | 9.671 |

### 执行耗时 `execution`（秒）

| Worker | P50 | P95 | P99 | avg |
|---:|---:|---:|---:|---:|
| 1 | 1.011 | 1.014 | 1.015 | 1.011 |
| 2 | 1.011 | 1.015 | 1.022 | 1.012 |
| 4 | 1.012 | 1.014 | 1.015 | 1.012 |

### 端到端 `total`（秒）

| Worker | P50 | P95 | P99 | avg |
|---:|---:|---:|---:|---:|
| 1 | 20.460 | 38.741 | 40.772 | 20.980 |
| 2 | 10.262 | 19.332 | 20.340 | 10.680 |
| 4 | 5.713 | 10.280 | 10.683 | 5.860 |

补充观察：

- 运行中任务数分别稳定在 1 / 2 / 4，与 Worker 配置一致。
- 每轮 `claimed_total` 与 `completed_total{status="succeeded"}` 均为 40；`execution_duration` 总和约 40s（40 × 1s Fake 延迟）。
- `execution` 的 P95/P99 三档均约 1.01–1.02s；下降的是批次墙钟时间以及 `queue_wait` / `total` 的 P95/P99。

## 实际结果

1. **不同 Worker 数量下的实际吞吐**

```text
1 Worker：0.973 task/s（总耗时 41.131s）
2 Worker：1.934 task/s（总耗时 20.688s）
4 Worker：3.598 task/s（总耗时 11.117s）
```

2. **吞吐是否接近线性增长**

相对 1 Worker：2 Worker 约 **1.99×**，4 Worker 约 **3.70×**。在当前 Fake 延迟主导的负载下接近线性；4 Worker 未达到理想 4×，主要受任务提交、轮询间隔和调度开销影响，不属于领取锁打满。

3. **最先出现的瓶颈**

当前瓶颈是 **Worker 数量与 1s 执行延迟**，不是 MySQL 领取竞争：

- `execution` P50/P95/P99 三档几乎不变（约 1.01–1.02s）；
- `queue_wait` P95 随 Worker 增加近似减半（37.7s → 18.3s → 9.3s），P99 同样下降（39.8s → 19.3s → 9.7s）；
- `total` P95 同步下降（38.7s → 19.3s → 10.3s），说明端到端尾延迟改善来自排队缩短；
- 运行中可见 `running` 顶满对应 Worker 数，队列被更快抽干。

4. **当前是否有理由引入 Redis 或消息队列**

**没有。** 本实验未观察到“增加 Worker 后吞吐不再提升”或领取路径成为主耗时。尾延迟（P95/P99）改善与 Worker 扩容一致，且执行侧尾延迟稳定在 Fake 延迟附近。在没有更短延迟、更高并发下的反证前，继续使用 MySQL 队列。

## 结论

在 MySQL + `llm_analysis` Fake 延迟 1s、每轮 40 个任务的条件下，将任务级 Worker 从 1 增至 2、4，批次完成时间从约 41s 降至约 21s、11s，吞吐接近线性提升；`queue_wait` / `total` 的 P95、P99 随 Worker 增加近似减半，而 `execution` 的 P95/P99 保持约 1.01s。实验证明 TaskPulse 的任务级并发配置有效，当前性能证据尚不足以支持引入 Redis 或其他消息队列。
