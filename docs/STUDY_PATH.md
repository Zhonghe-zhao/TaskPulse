# 学习路径

项目围绕一条主链路开发：

```text
提交 URL 批次
→ 创建异步任务
→ Worker 原子领取任务
→ Executor 有界并发访问 URL
→ 对结果和错误进行分类
→ 更新任务进度
→ 重试临时错误
→ 持久化并观察执行过程
```

## 第一阶段：领域模型

首先理解系统中的核心概念：

- Task：一个批次任务
- Workflow：任务的执行类型
- Status：任务当前所处的生命周期阶段
- Event：任务执行过程中发生的事情
- Progress：向用户展示的完成进度

当前版本没有独立 `TaskItem`。只有在每个 URL 需要独立领取、重试和查询时，再决定是否引入子任务模型。

对应代码：

```text
internal/domain
```

需要理解的后端概念：

- 领域建模
- 状态机
- 合法状态流转
- JSON 输入输出边界
- 单元测试

这一阶段的重要性：

> 任务基础设施必须让执行过程变得可见、可控制。第一步就是明确任务的生命周期。

## 学习原则

不要把技术当作清单背诵，而要跟随 URL 检测场景暴露的问题学习：

```text
多个 goroutine 同时访问 map
→ 学习 Mutex、RWMutex 和 Repository 边界

HTTP 请求缓慢且耗时不可预测
→ 学习 context 超时和连接复用

同时发出过多请求
→ 学习有界 Worker Pool 和背压

网络出现临时故障
→ 学习错误分类、重试和指数退避

Worker 执行过程中崩溃
→ 学习持久化、租约和任务恢复

不知道系统瓶颈在哪里
→ 学习指标、性能分析和压力测试
```

## 开发顺序

### 第一步：内存存储

实现并发安全的 Task Store 和 Event Store。

学习：

- interface
- map
- sync.RWMutex
- 错误定义
- 并发单元测试

### 第二步：任务 API

实现任务创建和查询接口。

学习：

- net/http
- JSON 编解码
- 参数校验
- Handler、Service、Repository 分层

### 第三步：Worker Pool

使用有界队列和固定数量 Worker 执行模拟任务。

学习：

- goroutine
- channel
- context
- WaitGroup
- select
- 优雅关闭

### 第四步：真实 URL 检测

访问 URL 并记录状态码、耗时、重定向和标题。

学习：

- HTTP Client
- Transport 和连接池
- 超时
- 重定向
- HTML 解析
- 错误分类

### 第五步：MySQL 持久化

将内存存储和队列替换为 MySQL 8。

学习：

- 表设计和索引
- 事务
- 行锁
- `FOR UPDATE SKIP LOCKED`
- 并发领取任务

### 第六步：可靠性

实现重试、租约、幂等和崩溃恢复。

学习：

- 指数退避
- 至少执行一次
- 重复执行
- Worker 租约
- 故障恢复

### 第七步：工程验证

加入监控、压测和故障测试。

学习：

- 结构化日志
- Prometheus
- pprof
- k6
- Docker Compose
- 故障注入
