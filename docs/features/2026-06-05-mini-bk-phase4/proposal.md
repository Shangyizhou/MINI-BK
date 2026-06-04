# Mini-BK ResourceOps Phase 4 Proposal

## Summary

在 Phase 3 节点管理与远程执行基础上，引入 etcd 作为控制面一致性存储，实现多 Scheduler 高可用调度。消除 Scheduler 单点故障，支持多个 Server 实例通过 Leader Election + CAS 协同工作。

## Goal

实现多 Scheduler 高可用调度架构，通过 etcd 提供 Leader Election、CAS 防重复调度、配置热更新和服务发现。

## Scope

1. **etcd 连接与配置**: etcd 连接管理、优雅降级
2. **Leader Election**: 基于 etcd concurrency 的 Leader 选举，竞选成功者执行调度
3. **NodeManager 迁移到 etcd**: 注册时写 PostgreSQL + etcd Lease；心跳续约；Watch 事件检测离线
4. **调度防重复抢占 (CAS)**: dispatch 前通过 etcd 事务 CAS 抢占任务 key
5. **服务发现**: etcd Watch `/nodes/` 前缀实时更新节点缓存
6. **Scheduler 集成 Leader Election**: 非 Leader Scheduler 不执行调度，失去 Leader 后自动重选
7. **配置热更新**: ConfigWatcher 监听 `/config/scheduler/` 前缀动态更新运行时配置
8. **Docker Compose**: 新增 etcd 服务，支持多 Server 实例

## Non-Goals

- 不做 etcd 集群部署（Docker Compose 单实例）
- 不做前端（纯 API）
- 不做持久化配置管理（配置热更新仅运行时生效）

## User-Facing Outcome

1. Server 多实例高可用部署，Leader 宕机后其他实例自动接管
2. 任务分配不重复（CAS 防抢占）
3. 配置热更新无需重启服务
4. etcd 不可用时优雅降级到单机模式

## Acceptance Criteria

| # | 标准 | 验证方式 |
|---|------|----------|
| 1 | etcd 可用时启动 Leader Election，不可用时降级到单调度器 | 代码审查 + 手动验证 |
| 2 | 同一时间只有一个 Scheduler 执行 tick 调度 | Leader Election 单元测试 |
| 3 | 多个 Scheduler 同时 dispatch 不重复分配任务 | CAS 抢占单元测试 |
| 4 | 节点心跳丢失后 etcd Lease 自动过期标记 OFFLINE | NodeManager 单元测试 |
| 5 | etcd Watch 配置变更实时反映到 Scheduler 运行时参数 | ConfigWatcher 审查 |
| 6 | Docker Compose 同时启动 etcd + PostgreSQL + Redis + Server + Agent | docker-compose up |
| 7 | 两个二进制编译通过，go vet 无警告 | 编译验证 |

## Risks

1. **etcd 单点**: Docker Compose 中 etcd 为单实例，生产应部署 etcd 集群
2. **Leader 切换延迟**: Leader 宕机后最多 10s（Session TTL）完成重新选举
3. **配置热更新未持久化**: 动态配置在 etcd 重启后丢失
4. **CAS 租约过期**: 长时间任务可能因 Claim key 租约过期被其他 Scheduler 抢占
