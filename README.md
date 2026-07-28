# TaskPulse

TaskPulse 是一个基于 Go 开发的可靠异步任务执行系统。

这个项目用于学习和解决长耗时、批量任务背后的后端工程问题：

- 有界并发
- 任务状态机
- 超时与取消
- 重试与幂等
- 进程崩溃后的任务恢复
- 任务进度追踪
- 可观测性与性能分析

TaskPulse 第一阶段不会把自己包装成万能工作流平台。我们先通过一个真实任务验证系统，再根据实际遇到的问题抽象通用基础设施能力。

项目的正式问题定义、边界和完成标准见：

- [文档索引](docs/README.md)
- [TaskPulse 项目章程](docs/PROJECT_CHARTER.md)
- [最小可行版本计划](docs/MVP.md)
- [系统架构](docs/ARCHITECTURE.md)
- [学习路径](docs/STUDY_PATH.md)

## 第一个真实场景：URL Inspector

TaskPulse 的第一个上层应用是“批量 URL 检测与网页元数据采集”。

用户提交一批 URL，系统执行：

```text
创建批次任务
→ 拆分为多个 URL 子任务
→ Worker 并发访问 URL
→ 记录状态码、响应时间、重定向、网页标题和错误
→ 对临时错误进行重试
→ 持续更新任务进度
→ 保存最终检测报告
```

这个场景会自然产生 TaskPulse 需要解决的问题：

- 每个网络请求的耗时不可预测
- 部分请求可能超时
- 有些错误可以重试，有些错误不能重试
- 必须限制并发数量，避免耗尽资源
- 单个 URL 失败不能导致整个批次失败
- 用户需要查看批次进度和每个 URL 的结果
- Worker 中断后，未完成任务需要恢复

## 项目边界

项目包含两层：

```text
TaskPulse Core
  负责任务生命周期、存储、排队、Worker、重试、租约、事件和监控

URL Inspector
  负责 URL 校验、HTTP 请求、网页元数据提取和结果展示
```

URL Inspector 用于证明 TaskPulse Core 确实能够解决真实问题。

TaskPulse Core 中不能出现 URL 特有的业务规则。

MemoBridge 不是 TaskPulse 第一版的依赖。未来 MemoBridge 真正出现批量链接检测、大型导出或 LLM 批处理需求时，再考虑接入。

## 本地运行

真实运行默认使用 MySQL。`compose.yaml` 第一次创建数据卷时会自动执行初始化迁移。

```powershell
docker compose up -d
```

应用直接读取进程环境变量，不会自动加载 `.env.example`。启动前需要在当前 PowerShell 设置 `MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_DATABASE` 等变量，然后运行：

```powershell
$env:TASKPULSE_STORAGE="mysql"
go run ./cmd/taskpulse
```

服务启动日志必须出现：

```text
TaskPulse storage backend: mysql
TaskPulse HTTP server listening on :8080
```

只有单元测试或不需要持久化的临时调试才显式使用：

```powershell
$env:TASKPULSE_STORAGE="memory"
go run ./cmd/taskpulse
```

MySQL 连接失败时应用直接退出，不会静默退回内存存储。

## 开发演进路线

```text
内存存储和内存队列
→ MySQL 8 持久化
→ 使用 FOR UPDATE SKIP LOCKED 实现 MySQL 任务领取
→ 加入租约和崩溃恢复
→ 加入指标、故障测试和压力测试
→ 根据测试结果判断是否需要 Redis Stream
```

## 第一版不做什么

- 可视化工作流编辑器
- 动态工作流 DSL
- 多 Agent 协作
- 插件市场
- 为展示技术而强行加入 Kafka
- 在单机系统尚未测量前引入 Kubernetes
- 强行接入 MemoBridge
