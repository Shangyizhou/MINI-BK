# Mini-BK ResourceOps 文档

> 文档包含两类信息：当前项目上下文（每次会话必读）和按需归档的特性文档。

## 新会话阅读顺序

1. [`context.md`](context.md) — 当前架构状态、项目方向和约束
2. [`history.md`](history.md) — 已完成工作的变更记录
3. 如需继续某个特性，阅读 `docs/features/<date>-<topic>/` 下的文件

## 特性文档规范

中大型特性按以下顺序产出文档：

1. `proposal.md` — 为什么要做、做什么、不做、验收标准
2. `design.md` — 总体方案、模块职责、数据模型、接口设计
3. `plan.md` — 分步实现计划（可引用 superpowers 计划）

小改动只更新 `context.md` 和 `history.md`。

## 已有特性

- [2026-06-01 Mini-BK Phase 1](features/2026-06-01-mini-bk-phase1/proposal.md) — 单机版资源任务平台
