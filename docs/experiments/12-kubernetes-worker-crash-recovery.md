# 12: Kubernetes Worker 崩溃恢复实验

## 问题

验证外部 `llm-worker` 运行在 Kubernetes Pod 中时，Worker Pod 被删除或崩溃后，系统能否恢复正在执行的任务。

本实验同时观察两套恢复机制：

```text
Kubernetes Deployment
  负责重新创建被删除的 Worker Pod

TaskPulse lease
  负责发现旧 Worker 已失去租约，并允许新 Worker 重新领取任务
```

## 环境

- Docker Desktop Kubernetes
- 两个 Kubernetes 节点
- MySQL StatefulSet
- TaskPulse API Deployment，2 个副本
- 外部 `llm-worker` Deployment，2 个副本
- Worker lease duration：30s
- Fake LLM delay：30s

## 前置条件

确认资源已经启动：

```powershell
kubectl get pods -n taskpulse -o wide
```

预期至少包含：

```text
mysql-0                 Running
taskpulse-xxxxx         Running
taskpulse-yyyyy         Running
llm-worker-xxxxx        Running
llm-worker-yyyyy        Running
```

为本机访问 API 建立端口转发。集群内部仍然使用 `8080`，本机使用 `18080`：

```powershell
kubectl port-forward -n taskpulse svc/taskpulse 18080:8080
```

## 实验步骤

### 1. 延长外部 Worker 的执行时间

```powershell
kubectl set env deployment/llm-worker -n taskpulse TASKPULSE_LLM_FAKE_DELAY=30s
kubectl rollout status deployment/llm-worker -n taskpulse
```

延长执行时间是为了在任务处于 `running` 时删除 Worker Pod，使旧 Worker 无法继续发送心跳。

### 2. 创建任务

```powershell
$body = @{
  workflow = "llm_analysis"
  input = @{
    subject = "Kubernetes worker recovery"
    notes = @("Pod", "lease", "Deployment")
    goal = "verify worker crash recovery"
  }
  max_retries = 3
} | ConvertTo-Json -Depth 5

$task = Invoke-RestMethod `
  -Method Post `
  -Uri "http://localhost:18080/tasks" `
  -ContentType "application/json" `
  -Body $body

$task
```

等待任务进入 `running`：

```powershell
Invoke-RestMethod "http://localhost:18080/tasks/$($task.id)"
```

记录任务的：

- `status`
- `version`
- `retry_count`
- `lease_owner`
- `lease_expires_at`

### 3. 删除正在执行任务的 Worker Pod

```powershell
$workerPod = kubectl get pod -n taskpulse -l app=llm-worker -o jsonpath="{.items[0].metadata.name}"
kubectl delete pod -n taskpulse $workerPod
```

### 4. 观察 Kubernetes 自动恢复 Pod

```powershell
kubectl get pods -n taskpulse -l app=llm-worker -o wide -w
```

预期现象：

```text
旧 Worker Pod     Terminating
新 Worker Pod     Pending
新 Worker Pod     ContainerCreating
新 Worker Pod     Running
```

Deployment 的副本数保持为 2，因此删除一个 Pod 后，Kubernetes 会创建新的 Pod 补齐副本数。

### 5. 观察 TaskPulse 恢复任务

```powershell
Invoke-RestMethod "http://localhost:18080/tasks/$($task.id)"
Invoke-RestMethod "http://localhost:18080/tasks/$($task.id)/events"
```

预期生命周期：

```text
queued
  -> running
  -> 旧 Worker 停止发送心跳
  -> lease_expires_at 到期
  -> 新 Worker 重新 claim
  -> succeeded
```

由于重新领取是一次新的执行尝试，任务的 `version` 和 `retry_count` 应发生相应变化。旧 Worker 使用旧版本提交结果时，应被 TaskPulse 拒绝，不能覆盖新 Worker 的结果。

## 需要记录的证据

```powershell
kubectl get pods -n taskpulse -o wide
kubectl get deployment -n taskpulse
kubectl logs -n taskpulse deployment/llm-worker --since=5m --timestamps
Invoke-RestMethod "http://localhost:18080/tasks/$($task.id)"
Invoke-RestMethod "http://localhost:18080/tasks/$($task.id)/events"
```

重点记录：

- 被删除的 Pod 名称；
- 新创建的 Pod 名称；
- 旧租约的过期时间；
- 新 Worker 重新领取任务的时间；
- 任务最终状态；
- 任务事件顺序；
- 任务是否出现重复成功写入。

## 结果

### 第一次故障注入结果

本次任务 ID：`task_1786019229701647379_4929`。

已观察到：

- Kubernetes 成功重新创建被删除的 `llm-worker` Pod；
- TaskPulse 成功记录 `task_recovered` 事件；
- 任务没有永久停留在 `running`；
- 任务最终进入 `failed`，`retry_count=3`。

关键事件顺序为：

```text
task_created
task_started
task_retrying
task_retry_started
task_recovered
task_retrying
task_retry_started
task_failed
```

失败原因是：

```text
external_worker_error:
Post /worker/tasks/{task_id}/heartbeat: context canceled
```

这暴露出外部 Worker 的一个问题：Pod 被 Kubernetes 终止时，Worker 收到 `context.Canceled`，却把进程终止或租约失效当成了普通业务失败，并消耗了任务的重试次数。

因此，本次实验已经证明了 TaskPulse 的租约恢复机制有效，但还没有证明任务可以在 Worker Pod 删除后最终成功。

### 修正

外部 Worker 现在遇到 `context.Canceled` 时不再调用 `/fail`：

```text
context.Canceled
  -> Worker 停止上报结果
  -> TaskPulse 等待 lease 到期
  -> 新 Worker 重新领取任务
```

只有真正的业务错误，例如 LLM 限流或模型服务不可用，才进入统一重试流程。

修正后需要重新执行本实验，并确认最终状态为 `succeeded`。

### 第二次验证结果

2026-08-07，修正代码并正确更新 Kubernetes Worker 镜像后，实验重新执行成功。

本次成功验证的关键操作是：

```powershell
docker build --no-cache `
  --build-arg APP=llm-worker `
  -t taskpulse-llm-worker:dev .

kubectl rollout restart deployment/llm-worker -n taskpulse
kubectl rollout status deployment/llm-worker -n taskpulse
```

成功结果：

- Worker Pod 被删除后，Deployment 自动创建新的 Worker Pod；
- TaskPulse 通过租约过期发现旧 Worker 已失效；
- 新 Worker 能够重新领取任务；
- 任务最终成功完成；
- 旧 Worker 的 `context canceled` 没有再被错误上报为业务失败。

本次还发现了两个部署操作问题：

1. `kubectl rollout restart deployment/llm-worker` 默认操作 `default` 命名空间，必须增加 `-n taskpulse`。
2. Dockerfile 使用 `APP` 构建参数。构建外部 Worker 必须指定 `--build-arg APP=llm-worker`，否则默认构建的是 `cmd/taskpulse`。

这两个问题会导致源码已经修改，但 Kubernetes 仍然运行旧 Worker，或者镜像中根本不是外部 Worker 程序。

## 结论

本实验用于说明 Kubernetes 和 TaskPulse 的职责边界：

```text
Kubernetes 负责进程级恢复：重新创建 Worker Pod。
TaskPulse 负责任务级恢复：通过租约发现执行者失效，并重新分配任务。
```

只有两者同时生效，Worker Pod 被删除后，正在执行的任务才不会永久停留在 `running` 状态。

本次最终验证结果表明：Kubernetes 负责恢复 Worker 进程，TaskPulse 负责恢复任务租约，外部 Worker 正确处理 Context 生命周期并提交最终结果，三者可以共同完成故障恢复闭环。

## 当前边界

- 本实验使用单副本 MySQL，不代表 MySQL 已具备高可用能力；
- Kubernetes Secret 仅用于本地学习，不适用于生产环境；
- 任务恢复依赖 lease 到期，不能保证任务只执行一次；
- 外部业务写回仍然需要幂等设计；
- 当前实验验证的是 Worker Pod 恢复，不是节点级故障恢复。
