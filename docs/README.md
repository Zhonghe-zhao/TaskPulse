# TaskPulse 文档索引

本文档目录按“目标、当前事实、决策、计划、证据”分工，避免把未来设想写成已经实现的能力。

## 文档职责

| 文档 | 回答的问题 | 更新时机 |
|---|---|---|
| [PROJECT_CHARTER.md](PROJECT_CHARTER.md) | 项目为什么存在，边界和成功标准是什么 | 项目定位或边界改变时 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 当前系统由什么组成，模块如何依赖 | 架构或运行链路改变时 |
| [MVP.md](MVP.md) | 当前阶段准备实现什么，完成标准是什么 | 里程碑推进时 |
| [STUDY_PATH.md](STUDY_PATH.md) | 开发过程中需要掌握哪些知识 | 学习重点改变时 |
| [adr/](adr/) | 为什么选择某个重要方案 | 作出或替换架构决策时 |
| [experiments/](experiments/) | 问题如何复现，方案如何验证 | 完成实验、压测或故障测试时 |

## ADR 索引

| 编号 | 决策 | 状态 |
|---|---|---|
| [ADR-0001](adr/0001-use-mysql-as-system-of-record.md) | 使用 MySQL 8 作为任务状态的持久化真相源和第一版持久化队列 | 已接受，待实现 |

## 维护规则

1. 当前代码没有实现的能力必须标记为“计划”或“目标”。
2. 重要技术选型先写 ADR，再进入实现。
3. ADR 被替换时不删除旧文件，而是标记为“已废弃”并链接新 ADR。
4. 每个工程问题按“现象、复现、决策、验证、边界”记录。
5. README 只做入口；详细设计只保留一个权威位置，避免重复维护。

## 当前文档状态

- 当前可运行版本：内存 Store、HTTP API、单任务 Worker、URL 有界并发执行器。
- 下一阶段已决策：引入 MySQL 8 持久化。
- Redis Streams、Prometheus、Docker Compose 和 Kubernetes 仍属于后续计划，需单独决策或实验支持。
