# ADR-0009：先用静态 LLM Analysis Workflow 承载 Agent/LLM 任务

- 状态：Accepted
- 日期：2026-07-30

## 背景

TaskPulse 已经通过 `url_check` 验证了任务创建、领取、执行、终态提交、租约、重试、
幂等和取消等通用能力。但如果系统长期只有 URL 检测，很容易被误解成一个批量抓取工具，
不能证明它能承载 Agent/LLM 这类上层智能任务。

同时，当前阶段不适合直接实现动态工作流、多 Agent 协作、任意工具调用或插件系统。
这些能力会扩大边界，掩盖我们真正要训练的后端基础能力：任务生命周期、可靠执行、
错误分类、重试和可观测性。

## 决策

新增一个静态 workflow：

```text
workflow = llm_analysis
```

它表示“对一组笔记或资料进行 LLM 分析，并生成结构化结果”的长耗时任务。

第一版只定义：

- `llm_analysis.Input`
- `llm_analysis.Output`
- 可替换的 `llm_analysis.Client`
- `FakeClient`
- `Executor`

`Executor` 只负责：

```text
解析任务输入
→ 校验 Prompt 边界
→ 调用 LLM Client
→ 将输出编码为 JSON
→ 将 Provider 错误分类为 transient/permanent
```

TaskPulse Core 仍然负责：

```text
任务创建
→ MySQL 持久化排队
→ Worker 领取
→ 租约心跳
→ 重试调度
→ 失败/成功终态提交
→ 事件记录
```

## 为什么先用 FakeClient

FakeClient 不是为了假装已经接入大模型，而是为了先稳定系统边界：

- 不依赖外部 API Key；
- 不受网络、额度和模型波动影响；
- 可以稳定测试 Executor 与 Worker 的集成；
- 后续接入真实 Provider 时，只替换 Client 实现，不改 Worker 和 Store。

## 暂不做

- 动态 Agent Workflow；
- 多 Agent 协作；
- 任意代码执行；
- 流式 Token 推送；
- RAG 检索；
- 插件市场。

这些能力只有在静态 `llm_analysis` 已经暴露出真实问题后再讨论。

## 后果

TaskPulse 开始从“URL 检测执行器”扩展为“通用任务运行时”。

面试中可以明确说明：

> URL Check 用来验证网络批处理；LLM Analysis 用来验证受限流、长耗时、成本敏感的智能任务。
> 两者共用同一套任务状态机、Worker、MySQL 队列、租约、重试和事件模型。

后续真实 Provider 接入时，需要补充：

- API Key 配置；
- HTTP Client 超时；
- 429/5xx/timeout 到 transient error 的映射；
- 400/401/403 到 permanent error 的映射；
- 模型名、耗时、Token 用量记录；
- LLM workflow 级并发限制。
