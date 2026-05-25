# OpsEngine 文档中心

本目录为项目技术文档，与根目录 [README.md](../README.md)（快速上手）互补。

## 文档索引

| 文档 | 说明 | 适合读者 |
|------|------|----------|
| [introduction.md](./introduction.md) | **项目完整介绍**：定位、概念、数据模型、内置节点、数据目录 | 新用户、产品/运维 |
| [architecture.md](./architecture.md) | **架构与调用分析**：分层、启动链路、执行生命周期、Frame 树、事件流 | 架构师、全栈 |
| [node-development.md](./node-development.md) | **节点开发手册**：Go 节点实现、注册、端口、ConfigSchema、前端对接 | 节点/插件开发者 |
| [source-reading.md](./source-reading.md) | **源码阅读说明**：推荐阅读顺序、关键文件、调试与测试入口 | 贡献者 |
| [phase8-13-plan.md](./phase8-13-plan.md) | **迭代计划**（Phase 8–13） | 维护者 |
| [execution-ux-plan.md](./execution-ux-plan.md) | **执行体验改进**：白屏修复（已完成）、调用栈侧栏、详情页稳健性 | 前端 / 全栈 |

## 建议阅读路径

```
新接触项目
  └─ introduction.md → README 快速开始 → 跑通 make dev

要改引擎 / 加节点
  └─ architecture.md（执行流）→ node-development.md → source-reading.md

要改前端画布 / 执行 UI
  └─ architecture.md（事件与 Frame）→ execution-ux-plan.md → source-reading.md（前端章节）
```

## 文档约定

- 代码路径均相对于仓库根目录 `OpsEngine/`。
- 文中 **Exec 流** 指控制流端口（`exec_in` / `exec_out`），**Data 流** 指数据端口求值。
- 与实现不一致时，以源码为准；欢迎提 Issue 修正文档。
