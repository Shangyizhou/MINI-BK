# Mini-BK ResourceOps Phase 3 Proposal

## Summary

在 Phase 2 基础上引入 Agent 分布式执行，Agent 注册节点、心跳保活、gRPC 远程执行，Server 根据节点资源智能调度任务。

## Goal

实现 Agent 分布式执行能力，支持多节点注册、心跳保活、节点选择调度和远程任务执行。

## Scope

1. **Agent 注册**: Agent 启动时通过 gRPC Register 向 Server 注册节点信息
2. **心跳上报**: Agent 每 10 秒上报 CPU/内存/磁盘/负载等资源快照
3. **节点离线检测**: Server 30 秒未收到心跳标记 OFFLINE
4. **节点管理 API**: 列表、详情、Drain、Uncordon
5. **节点标签**: 支持标签匹配调度（linux, gpu, ssd, build, test, prod）
6. **任务扩展**: NodeSelector JSONB 字段 + AssignedNodeID 字段
7. **调度算法**: 三层筛选（标签匹配 → 资源过滤 → 最少负载）
8. **gRPC PullTask**: Agent 轮询拉取任务并执行
9. **结果回传**: Agent ReportResult 上报执行结果
10. **Docker Compose**: 编排 Server + Agent + PostgreSQL + Redis

## Non-Goals

- 不做 etcd（节点状态存 PostgreSQL + Redis 缓存）
- 不做前端（纯 API）

## User-Facing Outcome

1. Agent 注册后 Server 节点列表可见状态 ONLINE
2. 任务可根据标签选择器调度到匹配节点
3. Agent 心跳超时 30s 后标记 OFFLINE
4. 节点 Drain 后新任务不调度到该节点

## Acceptance Criteria

| # | 标准 |
|---|------|
| 1 | Agent 注册后 Server 节点列表可见状态 ONLINE |
| 2 | 任务可根据标签选择器调度到匹配节点 |
| 3 | Agent 心跳超时 30s 后标记 OFFLINE |
| 4 | 任务取消信号在 5s 内到达 Agent 并 kill 进程 |
| 5 | Server 重启后能恢复与已知 Agent 的连接 |
| 6 | 节点 Drain 后新任务不调度到该节点 |

## Risks

1. **Agent 无状态**: 重启后重新注册，Server 清理旧连接
2. **任务超时**: 由 Agent 本地控制（context.WithTimeout），不依赖 Server 持续通信
3. **日志流**: 当前使用 gRPC 双向流 + Agent 推送日志块到 Server
4. **Agent 目录隔离**: 每个任务在 `<workdir>/tasks/<task_uid>/` 下执行
