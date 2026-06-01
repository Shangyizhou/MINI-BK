# Mini-BK ResourceOps Phase 1 Proposal

## Summary

构建单机版资源任务平台，跑通任务调度最小闭环：提交任务 → 排队 → 调度 → 执行 → 查日志。技术栈：Go + Gin + PostgreSQL + 本地进程管理（goroutine/channel + os/exec）。

## Why Now

项目从零开始，初始阶段不应引入分布式复杂度（etcd、Redis、gRPC、Agent），否则会陷入多组件半吊子状态。一期聚焦理解调度平台的核心：**任务状态机 + 队列 + 资源账本**。

## Scope

1. 用户创建任务（command, cpu/memory/timeout/workdir/env/priority）
2. 系统维护本机资源池，资源够则执行，不够则 Pending
3. 任务状态机：Created → Pending → Running → Success/Failed/Canceled
4. 任务列表、详情、执行日志、取消任务、重跑任务
5. 并发控制（buffered channel 信号量，最多同时跑 N 个任务）
6. 超时 kill（context.WithTimeout）
7. 执行历史落库（PostgreSQL）
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
4. 通过 Docker Compose 一键启动（Server + PostgreSQL）

## Technical Impact

新增模块：
- `cmd/server/` — 程序入口
- `internal/config/` — 配置管理（Viper）
- `internal/model/` — 数据模型与状态机
- `internal/store/` — PostgreSQL 持久化
- `internal/executor/` — 进程执行器
- `internal/scheduler/` — 资源感知调度器
- `internal/service/` — 业务逻辑层
- `internal/api/` — Gin HTTP API
- `migrations/` — 数据库迁移脚本

## Risks

1. **并发安全**: Scheduler 和 Executor 共享任务状态，需要 store.Update 保证一致性
2. **超时检测延迟**: ticker 轮询方式检测超时，最坏延迟 = tick_interval（500ms）
3. **无认证**: API 无鉴权，仅适合内网或本地使用

## Acceptance Criteria

| # | 标准 |
|---|------|
| 1 | 创建任务后可在 PostgreSQL 查到状态 Created |
| 2 | Pending 任务在资源足够时自动变为 Running |
| 3 | `echo hello` 执行成功，状态 Success，stdout="hello\n" |
| 4 | 超时任务自动 kill，状态 Failed，error_message 含 "timeout" |
| 5 | 取消 Running 任务，进程被杀，状态 Canceled |
| 6 | 并发达到上限时新任务进入 Pending |
| 7 | GET /api/v1/tasks 返回分页列表 |
| 8 | GET /api/v1/tasks/:uid/log 返回 stdout/stderr |
| 9 | 服务重启后历史任务仍可查询 |
| 10 | 资源余量 API 返回正确的 CPU/内存可用量 |

## Follow-Up

- Phase 2: Redis 队列版任务平台（任务持久化队列、幂等、限流、实时日志流）
- Phase 3: Agent 分布式执行平台（gRPC、多节点调度、心跳）
