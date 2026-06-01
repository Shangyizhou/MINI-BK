# Mini-BK ResourceOps Phase 1 Proposal

## Summary

构建单机版资源任务平台，跑通任务调度最小闭环。技术栈：Go + Gin + PostgreSQL + 本地进程管理。

## Why Now

项目从零开始，一期不能引入分布式复杂度（etcd、Redis、gRPC、Agent），否则会陷入多组件半吊子状态。必须先理解调度平台的核心：**任务状态机 + 队列 + 资源账本**。

## Scope

1. 用户创建任务（command, cpu/memory/timeout/workdir/env/priority）
2. 系统维护本机资源池，资源够则执行，不够则 Pending
3. 任务状态机：Created → Pending → Running → Success/Failed/Canceled
4. 任务列表、详情、执行日志、取消任务、重跑任务
5. 并发控制（最多同时跑 N 个任务）
6. 超时自动 kill
7. 执行历史落库
8. 资源余量 API 和统计 API

## Non-Goals

- 不做分布式（单机执行）
- 不做前端
- 不做脚本库、定时任务
- 不做文件分发
- 不做权限系统
- 不做 Redis / etcd / gRPC

## User-Facing Outcome

1. REST API 可提交任务、查询状态、获取日志
2. 任务自动调度执行，超时自动 kill
3. 服务重启后历史任务可继续查询

## Risks

1. **并发安全**: Scheduler 和 Executor 共享任务状态，通过 store.Update 保证一致性
2. **超时检测延迟**: ticker 轮询检测超时，最坏延迟 = tick_interval（500ms）
3. **无认证**: API 无鉴权，仅适合内网或本地使用

## Acceptance Criteria

| # | 标准 | 验证方式 |
|---|------|----------|
| 1 | 创建任务后可在 PostgreSQL 查到状态 Created | 集成测试 |
| 2 | Pending 任务在资源足够时自动变为 Running | 集成测试 |
| 3 | `echo hello` 执行成功，状态 Success，stdout="hello\n" | 集成测试 |
| 4 | 超时任务自动 kill，状态 Failed，error_message 含 "timeout" | 集成测试 |
| 5 | 取消 Running 任务，进程被杀，状态 Canceled | 集成测试 |
| 6 | 并发达到上限时新任务进入 Pending | 集成测试 |
| 7 | GET /api/v1/tasks 返回分页列表 | 集成测试 |
| 8 | GET /api/v1/tasks/:uid/log 返回 stdout/stderr | 集成测试 |
| 9 | 服务重启后历史任务仍可查询 | 集成测试 |
| 10 | 资源余量 API 返回正确的 CPU/内存可用量 | 集成测试 |

## Follow-Up

- Phase 2: Redis 队列版任务平台（队列持久化、幂等、限流、实时日志流）
- Phase 3: Agent 分布式执行平台（gRPC、多节点调度、心跳）
