# 06：Worker 崩溃恢复实验

## 目的

验证 Worker 领取任务后突然退出时，任务不会永久停留在 `running`，而是会在租约到期后被新的 Worker 恢复领取。

这个实验验证的是 TaskPulse 最核心的基础设施保证：任务状态持久化、租约、过期接管和版本隔离。

## 环境

- 存储：MySQL
- Workflow：`llm_analysis`
- Fake LLM 延迟：`10s`
- Worker 租约：`5s`
- 建议使用两个独立的 TaskPulse 进程

## 步骤

1. 启动 MySQL，并确认初始化迁移已经执行。

2. 启动第一个 TaskPulse 进程：

```powershell
$env:TASKPULSE_STORAGE="mysql"
$env:TASKPULSE_LLM_FAKE_DELAY="10s"
$env:TASKPULSE_WORKER_LEASE_DURATION="5s"
go run ./cmd/taskpulse
```

3. 创建一个 `llm_analysis` 任务，并立即查询任务状态。日志应先出现：

```text
msg="task claimed" workflow=llm_analysis
```

任务状态应为 `running`，并且存在 `lease_expires_at`。

4. 在 Fake LLM 仍处于延迟期间终止第一个进程。不要删除 MySQL 数据。

5. 等待租约超过 5 秒后，启动第二个 TaskPulse 进程，使用相同的 MySQL 配置。

6. 查询任务事件。期望看到：

```text
task_created
task_started
task_recovered
task_succeeded
```

实际事件名称以当前领域模型返回为准，重点是第二个 Worker 必须以 recovery claim 重新领取任务。

## 观察点

- 第一个 Worker 退出后，任务不会被永久卡在 `running`。
- 租约未过期时，第二个 Worker 不能领取该任务。
- 租约过期后，第二个 Worker 可以领取该任务。
- 旧 Worker 的终态提交不能覆盖新 Worker 已经提交的结果。
- `/metrics` 中可以观察到新的 claim，以及必要时的 lease renewal/lost 指标。

## 结论

如果任务能够在 Worker 崩溃后恢复，说明 TaskPulse 不依赖单个进程内存保存执行状态，而是使用 MySQL 作为任务真相源，通过租约表达“当前 Worker 暂时拥有执行权”。

这不是 exactly-once 执行保证。租约过期前后可能存在重复执行窗口，因此业务执行器仍需要幂等写回；TaskPulse 通过版本条件更新避免旧 Worker 覆盖新状态。

## 当前边界

- 本实验只验证单个任务恢复。
- 尚未验证多个 Worker 的吞吐和竞争数据。
- Fake LLM 不代表真实 Provider 的网络行为。
- 运行中任务的主动取消仍是后续能力。
