# 实验 02：Application 层串联任务创建用例

## 问题

我们已经有了：

- `domain.Task`：任务是什么
- `TaskStore`：任务怎么保存
- `EventStore`：事件怎么记录

但外部系统还不能真正“创建一个任务”。

这个阶段要解决的问题是：

```text
一次“创建任务”请求，应该由谁负责协调？
```

## 初始风险

如果把所有逻辑都写在 HTTP handler 里：

```text
handler 直接 new Task
handler 直接调用 store.Create
handler 直接 append event
```

会导致：

- HTTP 层和业务编排耦合
- 以后 Worker、取消、重试都要改 handler
- 难以单独测试“创建任务”这个用例

## 当前方案

新增 `internal/application/TaskService`：

```text
CreateTask
GetTask
ListTaskEvents
```

`CreateTask` 的流程：

```text
校验输入
→ domain.NewTask
→ TaskStore.Create
→ EventStore.Append(task_created)
→ 返回任务
```

## 为什么需要 Application 层

| 层 | 回答的问题 |
|---|---|
| domain | 任务是什么，状态怎么流转 |
| store | 数据怎么保存和读取 |
| application | 一次用户操作用哪些步骤完成 |
| transport | HTTP 怎么接收和返回请求 |

Application 层不直接关心 HTTP，也不直接操作 map 或 SQL。

## 验证方式

```powershell
go test ./internal/application/...
```

测试覆盖：

- 成功创建任务
- 自动写入 `task_created` 事件
- 拒绝非法输入
- 查询不存在任务的事件时返回 `ErrTaskNotFound`

## 当前限制

- 如果 `TaskStore.Create` 成功但 `EventStore.Append` 失败，会留下没有事件记录的任务
- 第一版先接受这个限制，后续引入事务或补偿逻辑

## 后续结果

HTTP API 已实现为：

```http
POST /tasks
GET /tasks/{task_id}
GET /tasks/{task_id}/events
```

当前限制中的 Task/Event 非原子写入问题将在 MySQL 事务阶段解决，见 `docs/adr/0001-use-mysql-as-system-of-record.md`。
