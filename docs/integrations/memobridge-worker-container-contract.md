# MemoBridge Worker 容器接入契约

## 目标

本契约用于把 MemoBridge API、MemoBridge Worker 和 TaskPulse 放进同一个 Docker Compose 网络。
TaskPulse 不读取 MemoBridge 数据库，MemoBridge Worker 仍然拥有 SourceItem 和 SemanticProfile 的业务数据。

## MemoBridge 需要提供的镜像

MemoBridge 仓库需要新增 `Dockerfile.worker`，构建入口为：

```text
./cmd/memobridge-worker
```

Worker 镜像启动后必须以前台进程运行，并正确响应 `SIGTERM`，以便 Compose 和 Kubernetes 优雅停止。

## 必需环境变量

```text
DATABASE_URL=postgres://memobridge:memobridge_local@postgres:5432/memobridge?sslmode=disable
TASKPULSE_URL=http://taskpulse:8080
TASKPULSE_WORKER_ID=memobridge-worker-1
TASKPULSE_LEASE_DURATION=30s
TASKPULSE_POLL_INTERVAL=1s
TASKPULSE_HEARTBEAT_INTERVAL=10s
```

说明：

- `taskpulse:8080` 是 Compose 网络内地址，不是宿主机映射端口 `8085`。
- `127.0.0.1:8085` 只用于宿主机上的手工测试。
- `DATABASE_URL` 使用 Compose 服务名 `postgres`，不能写 `localhost`。
- `TASKPULSE_WORKER_ID` 多副本时必须唯一，可以使用 Pod 名或 Compose 容器名注入。

AI 配置由 MemoBridge 自己管理，例如：

```text
MEMOBRIDGE_AI_PROVIDER=fake
DEEPSEEK_API_KEY=
DEEPSEEK_MODEL=deepseek-chat
```

## 期望的联调拓扑

```text
宿主机
  ├─ http://127.0.0.1:8081  -> MemoBridge API:8080
  └─ http://127.0.0.1:8085  -> TaskPulse:8080

Compose 网络
  MemoBridge API  -> postgres:5432
  MemoBridge Worker -> postgres:5432
  MemoBridge Worker -> http://taskpulse:8080
```

## 验收顺序

1. 启动 PostgreSQL、MySQL 和 TaskPulse。
2. 启动 MemoBridge API 和 Worker。
3. 通过 MemoBridge API 创建 SemanticProfile 任务。
4. 确认 Worker 日志出现 `claimed`、`heartbeat`、`completed`。
5. 通过 TaskPulse 查询任务和事件。
6. 在 MemoBridge 查询对应 SemanticProfile。
7. 使用相同请求重复创建，确认返回原任务且 Worker 不重复执行。
8. 删除 Worker 容器，确认租约过期后任务被重新领取并最终完成。

## 本阶段不做

- 不引入 ZooKeeper、etcd、服务网格。
- 不让 TaskPulse 直接访问 MemoBridge PostgreSQL。
- 不把 SourceItem 正文、Prompt 或完整 LLM 输出放进 TaskPulse。
- 不把 Redis 或 Kafka 作为当前联调的前置依赖。
