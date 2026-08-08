# 13: Kubernetes Worker 扩缩容实验

## 问题

验证增加 Kubernetes 中 `llm-worker` Deployment 的副本数后，TaskPulse 是否能够让多个 Worker 并发领取任务，并观察吞吐量变化。

## 实验条件

- TaskPulse API 运行在 Kubernetes 中；
- MySQL 作为持久化任务队列；
- `llm-worker` 通过外部 Worker 协议领取任务；
- 两次测试使用相同的任务负载和执行配置；
- 只改变 Worker 副本数量。

## 测试结果

### 2 个 Worker

```text
duration_seconds = 601.5954401
throughput       = 0.0664898656701105 tasks/s
```

### 4 个 Worker

```text
duration_seconds = 300.9931894
throughput       = 0.13289337237077 tasks/s
```

## 对比

```text
耗时缩短比例：约 2.00 倍
吞吐量提升比例：约 2.00 倍
```

计算方式：

```text
601.5954401 / 300.9931894 ≈ 1.9987
0.13289337237077 / 0.0664898656701105 ≈ 1.9987
```

## 结果分析

在相同任务负载下，Worker 数量从 2 增加到 4：

```text
2 个 Worker 并发领取任务
→ 4 个 Worker 并发领取任务
→ MySQL 事务和任务领取逻辑避免重复领取
→ 任务总处理时间接近减半
```

这说明当前任务执行阶段主要可以并行化，TaskPulse、MySQL 任务领取和外部 Worker 协议没有明显阻止并发扩展。

## 技术结论

Kubernetes 只负责调整 Worker Pod 副本数量：

```text
kubectl scale deployment/llm-worker --replicas=4
```

TaskPulse 负责保证多个 Worker 的任务竞争安全：

- 一个任务同时只能被一个有效 Worker 持有；
- Worker 通过租约证明自己仍然存活；
- 任务完成通过版本号防止旧 Worker 覆盖新结果；
- MySQL 事务保证并发领取的一致性。

因此，扩容能力不是 Kubernetes 单独提供的，而是：

```text
Kubernetes 副本管理
+ TaskPulse 并发领取
+ MySQL 事务与租约
```

共同实现的。

## 边界与后续问题

- 当前测试使用模拟 LLM 延时，不能代表真实模型服务吞吐；
- 当前只观察总耗时和吞吐量，还需要补充排队延迟、执行延迟和 P95/P99；
- Worker 数量继续增加后，可能出现 MySQL 连接、锁竞争或外部模型限流；
- 需要通过更大规模压测找到系统的扩展瓶颈。
