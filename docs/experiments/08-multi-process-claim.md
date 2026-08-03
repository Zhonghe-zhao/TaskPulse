# 08：多进程 Worker 竞争领取实验

## 目的

验证两个独立的 TaskPulse 进程连接同一个 MySQL 时，可以共同消费任务，并且同一个任务不会被两个进程同时有效领取。

## 环境

- 两个独立 TaskPulse 进程
- 相同 MySQL 数据库
- 每个进程启动 2 个 Worker
- `llm_analysis` Fake LLM 延迟：`3s`

## 步骤

在终端一启动：

```powershell
$env:TASKPULSE_STORAGE="mysql"
$env:TASKPULSE_WORKER_COUNT="2"
$env:TASKPULSE_LLM_FAKE_DELAY="3s"
go run ./cmd/taskpulse
```

在终端二启动相同配置的另一个进程：

```powershell
$env:TASKPULSE_STORAGE="mysql"
$env:TASKPULSE_HTTP_ADDR=":8081"
$env:TASKPULSE_WORKER_COUNT="2"
$env:TASKPULSE_LLM_FAKE_DELAY="3s"
go run ./cmd/taskpulse
```

两个进程使用不同的 HTTP 端口，但必须使用完全相同的 MySQL 配置。

然后创建至少 8 个任务，可以向任意一个进程的 API 发送请求：

```powershell
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/tasks" ...
```

观察两个终端的日志，任务应该由两个进程中的多个不同 `worker_id` 领取。

## 结论

当前完整 TaskPulse 进程同时包含 API 和 Worker；通过不同 HTTP 端口启动多个进程，已经可以验证跨进程任务竞争。后续如果需要独立扩缩容，再拆分 API 和 Worker 入口。

## 实际结果

- 两个独立 TaskPulse 进程使用不同 HTTP 端口启动。
- 两个进程连接同一个 MySQL 数据库。
- 两个进程中的 Worker 共同领取任务。
- 任务最终正常完成。
- 未观察到同一个任务被两个 Worker 同时有效领取。

因此，本实验验证了当前 MySQL 任务队列在跨进程场景下的领取互斥性。
