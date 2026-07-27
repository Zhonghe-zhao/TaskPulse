# 实验 01：并发安全的内存任务存储

## 问题

TaskPulse 创建任务后需要保存任务状态。第一版暂时不引入数据库，而是使用内存存储。

这个阶段要解决的问题是：

```text
多个 goroutine 同时创建、查询和更新任务时，如何保证内存状态正确？
```

## 初始风险

如果直接使用：

```go
map[string]*domain.Task
```

会有几个问题：

- map 并发读写会产生数据竞争
- 调用方可能修改 Store 内部保存的任务指针
- `json.RawMessage` 底层是字节切片，普通结构体拷贝仍然会共享底层数组
- 查询不存在和重复创建需要明确错误语义

## 当前方案

实现 `MemoryTaskStore`：

```text
sync.RWMutex 保护 map
Create 保存任务副本
Get 返回任务副本
Update 使用任务副本替换旧值
```

对 `json.RawMessage` 进行单独深拷贝：

```go
append(json.RawMessage(nil), raw...)
```

## 验证

已覆盖测试：

- 成功创建并查询任务
- 拒绝重复任务 ID
- 查询不存在任务返回 `ErrTaskNotFound`
- 更新任务
- 拒绝更新不存在任务
- 修改原始 Task 不影响 Store 内部数据
- 修改 Get 返回值不影响 Store 内部数据
- 多 goroutine 并发创建和读取
- 已取消 context 会中断 Store 操作

普通测试通过：

```text
go test ./...
```

## 当前限制

`go test -race ./...` 需要 cgo。当前 Windows 环境缺少 C 编译器 `gcc`，因此 race detector 暂时无法运行。

后续可选方案：

- 安装可用 C 编译器后运行 race detector
- 在 Linux / WSL 环境中运行 race detector
- 在 CI 中配置 race 测试

## 下一步

在 Store 之上实现应用服务：

```text
Create URL check task
Query task
Update task status
Append task events
```

