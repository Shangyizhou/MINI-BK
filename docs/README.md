# Mini-BK ResourceOps 文档

## 文档分类

| 类型 | 文件 | 作用 | 何时读 |
|------|------|------|--------|
| 会话恢复 | `context.md` | 当前架构状态、技术栈、约束、风险 | **每次新会话第一个读** |
| 变更记录 | `history.md` | 按时间倒序记录已完成的工作 | 了解项目进展 |
| 特性提案 | `features/<topic>/proposal.md` | 做什么、为什么、不做、验收标准 | 理解需求背景 |
| 特性设计 | `features/<topic>/design.md` | 模块概览 + API 速查，详细方案指向 superpowers | 快速了解设计要点 |
| 特性计划 | `features/<topic>/plan.md` | 任务清单 + 状态，详细步骤指向 superpowers | 跟踪进度 |
| 权威设计 | `superpowers/specs/<date>-<topic>-design.md` | Claude 生成的完整设计文档（数据模型、状态机、接口） | 深入了解实现细节 |
| 权威计划 | `superpowers/plans/<date>-<topic>-implementation.md` | Claude 生成的分步实现计划（含代码和 TDD 步骤） | 执行实现 |

## 设计文档的分工

- **`features/*/proposal.md`** — 自包含，记录需求和验收标准
- **`features/*/design.md`** — 概览 + 链接到 `superpowers/specs/` 的完整设计
- **`features/*/plan.md`** — 任务表 + 链接到 `superpowers/plans/` 的完整计划
- **`superpowers/specs/`** — Claude 生成的权威设计文档（数据模型、状态机、API 细节）
- **`superpowers/plans/`** — Claude 生成的分步实现计划（TDD 步骤、完整代码）

`features/` 是索引入口，`superpowers/` 是内容主体。

## 已有特性

- [2026-06-01 Mini-BK Phase 1](features/2026-06-01-mini-bk-phase1/proposal.md) — 单机版资源任务平台 ✅
