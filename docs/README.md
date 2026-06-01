# Mini-BK ResourceOps 文档

## 文档分类

| 类型 | 文件 | 作用 | 何时读 |
|------|------|------|--------|
| 会话恢复 | `context.md` | 当前架构状态、技术栈、约束、风险 | **每次新会话第一个读** |
| 变更记录 | `history.md` | 按时间倒序记录已完成的工作 | 了解项目进展 |
| 特性提案 | `features/<date>-<topic>/proposal.md` | 做什么、为什么、不做、验收标准 | 理解需求的背景和边界 |
| 特性设计 | `features/<date>-<topic>/design.md` | 怎么做的：架构、模块职责、数据模型、API | 理解实现方案 |
| 特性计划 | `features/<date>-<topic>/plan.md` | 分步任务清单，每步涉及的文件和关键逻辑 | 执行或 review 实现 |

## 新会话阅读顺序

1. **`context.md`** — 恢复项目上下文（架构、分层、数据流、验证命令）
2. **`history.md`** — 了解最近做了什么
3. 如需深入某个特性，进入 `features/<date>-<topic>/` 按 proposal → design → plan 顺序阅读

## proposal vs design 的区别

- **proposal** 回答 "该不该做" — 问题背景、范围边界、非目标、验收标准。不涉及技术方案
- **design** 回答 "怎么做" — 架构方案、模块职责拆分、数据模型、API 契约、并发模型。不重复 proposal 的内容

两者互补，不重复。

## 已有特性

- [2026-06-01 Mini-BK Phase 1](features/2026-06-01-mini-bk-phase1/proposal.md) — 单机版资源任务平台 ✅
