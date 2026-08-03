# 11：外部 Worker 协议实验

## 目的

验证业务执行逻辑可以独立于 TaskPulse 进程，通过 HTTP 协议领取任务、续租并提交结果。

## 协议

### 领取任务

```http
POST /worker/tasks/claim
```

```json
{
  "worker_id": "external-worker-1",
  "lease_duration": "30s"
}
```

成功响应是一个 `running` 任务，其中包含：

- `id`：任务 ID；
- `input`：业务输入；
- `version`：领取时的版本；
- `lease_expires_at`：租约过期时间。

没有可领取任务时返回 `204 No Content`。

### 续租

```http
POST /worker/tasks/{task_id}/heartbeat
```

```json
{
  "worker_id": "external-worker-1",
  "lease_duration": "30s"
}
```

### 成功

```http
POST /worker/tasks/{task_id}/complete
```

```json
{
  "worker_id": "external-worker-1",
  "version": 1,
  "output": {
    "summary": "analysis result"
  }
}
```

### 失败

```http
POST /worker/tasks/{task_id}/fail
```

```json
{
  "worker_id": "external-worker-1",
  "version": 1,
  "error_code": "provider_unavailable",
  "error_message": "provider returned 503"
}
```

## 业务 Worker 流程

```text
循环领取任务
→ 根据 workflow 选择业务处理器
→ 执行 LLM、文件处理或第三方 API 调用
→ 定期续租
→ 成功调用 complete
→ 失败调用 fail
```

业务 Worker 不访问 TaskPulse 数据库，也不负责修改 Task 状态。TaskPulse 负责租约、版本、状态转换和事件持久化。

## 版本隔离

外部 Worker 完成任务时必须携带领取响应中的 `version`。如果任务已经被取消、恢复领取或被其他 Worker 更新，版本不一致，TaskPulse 返回 `409 Conflict`，旧 Worker 不能覆盖新结果。

## 当前边界

- 当前协议面向受信任的本地 Worker，尚未加入认证和权限控制。
- 外部 Worker 的失败第一版直接进入 `failed`，还没有通过 HTTP 触发统一重试策略。
- 还没有独立的 `cmd/taskpulse-worker` 示例程序。
