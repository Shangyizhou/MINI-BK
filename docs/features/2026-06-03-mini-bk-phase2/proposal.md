# Mini-BK ResourceOps Phase 2 Proposal

## Summary

在 Phase 1 单机版基础上引入 Redis，实现队列持久化、幂等提交、实时日志流、接口限流和任务自动重试。Phase 2 不改变整体单体架构，Redis 作为可选组件，不可用时优雅降级。

## Goal

完成 Redis 集成，提供更可靠的任务调度和实时能力。

## Scope

1. **Redis 集成**: 连接管理、配置化、优雅降级
2. **任务队列持久化**: Redis List/SortedSet 作为任务队列，降级到 InMemQueue
3. **幂等提交**: 基于 SETNX 的 5 分钟去重窗口
4. **实时日志流**: Redis Stream + SSE 端点推送实时日志
5. **接口限流**: Redis INCR + TTL 实现 IP 级别限流
6. **任务自动重试**: 失败任务指数退避重试（默认最多 3 次）
7. **延迟任务**: PushDelayed 接口 + 时间轮询出队
8. **每日统计**: Redis Hash 记录每日统计
9. **Docker Compose**: 编排 PostgreSQL + Redis + Server

## Non-Goals

- 不做分布式调度
- 不做持久化队列的消息可靠性保证
- 不做限流策略的动态更新

## User-Facing Outcome

1. 相同任务重复提交被拒绝
2. 实时日志流实时推送
3. 失败任务自动重试
4. 接口限流避免滥用

## Acceptance Criteria

| # | 标准 | 验证方式 |
|---|------|----------|
| 1 | Redis 可用时使用 Redis 队列，不可用时降级到内存队列 | 单元测试 + 代码审查 |
| 2 | 相同命令+工作目录+环境变量在 5 分钟内重复提交被拒绝 | TestIdempotency |
| 3 | 失败任务自动重试，retry_count 递增，最终状态 failed | TestTaskRetry |
| 4 | SSE 端点实时推送日志行和完成事件 | TestSSELogStream |
| 5 | 超过限流阈值的请求返回 429 | TestRateLimit |
| 6 | 新创建的任务非终态 | TestDelayedTask |
| 7 | 每日统计 API 返回 submitted/success/failed | 手动验证 |
| 8 | Docker Compose 同时启动 PG + Redis + Server | docker-compose up |

## Risks

1. **Redis 不可用**: 核心功能降级，不影响 PG 中的数据
2. **限流偏差**: 非原子计数 + 并发请求可能导致少量超限
3. **重试风暴**: 默认重试间隔较低，大量失败任务可能导致重试风暴
