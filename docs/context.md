# Context

## 项目定位

**Mini-BK ResourceOps**：面向 Linux/容器资源的任务调度与运维管控平台。

借鉴蓝鲸体系的经典能力，用 Go 生态从零构建小型基础设施平台。核心价值是把 **资源、任务、节点、执行、日志、权限、调度、监控、流程** 串成一个闭环。

## 技术栈

| 层 | 选择 | 状态 |
|----|------|------|
| 语言 | Go 1.22+ | ✅ |
| HTTP 框架 | Gin | ✅ |
| 数据库 | PostgreSQL | ✅ |
| 进程管理 | os/exec + goroutine + channel | ✅ |
| 队列/缓存 | Redis | Phase 2 |
| 进程间通信 | gRPC + Protobuf | Phase 3 |
| 一致性存储 | etcd | Phase 4 |
| 前端 | React + Ant Design | Phase 3+ |
| 可观测性 | Prometheus + Grafana | Phase 7 |

## 当前阶段

**Phase 1 — 单机版资源任务平台** (已完成)

完成了最小闭环：提交任务 → 排队 → 调度 → 执行 → 查日志。

## 架构分层

```
cmd/server/          ← 入口：组装所有组件
internal/
├── api/             ← HTTP 层：Gin router + handler
├── service/         ← 业务逻辑层
├── scheduler/       ← 调度器：资源感知 ticker 循环
├── executor/        ← 执行器：os/exec 进程管理
├── model/           ← 数据模型 + 状态机
├── store/           ← 持久化层：PostgreSQL
└── config/          ← 配置管理：Viper
```

## 数据流

```
HTTP POST /api/v1/tasks
  → task_handler.go: bind JSON
  → task_service.go: CreateTask()
  → task_store.go: INSERT INTO tasks
  → scheduler.go: tick() 发现 Created 任务
  → 资源够? dispatch(task) : task.Status = Pending
  → executor.go: Run() → os/exec
  → completeTask() / failTask()
  → task_store.go: UPDATE tasks
```

## 存储职责

| 数据类型 | PostgreSQL | 说明 |
|----------|------------|------|
| 任务记录 | ✅ 主 | 所有任务的历史事实 |
| 执行日志 | ✅ | stdout/stderr 落库 |

Phase 2+ 将引入 Redis（队列/缓存）和 etcd（控制面）。

## 架构规则

### 状态机

```
Created → Pending → Running → Success/Failed/Canceled
```

- 终态（Success/Failed/Canceled）不可再流转
- 取消只能在非终态进行
- Scheduler 负责检查超时并标记 Failed

### 并发控制

- 使用 buffered channel 信号量限制同时执行的任务数
- 配置项: `scheduler.max_concurrent_tasks`（默认 10）

### 错误处理

- 不忽略 error，使用 `fmt.Errorf("context: %w", err)` wrap
- API 层返回统一的 `{"error": "..."}` 格式
- 结构化日志使用 `log/slog`

## 验证命令

```bash
# 单元测试
go test ./internal/... -v -count=1

# 编译
go build ./cmd/server

# 静态检查
go vet ./...
```

## 已知风险

1. **无分布式能力**: 当前单机执行，服务重启会丢失内存中的执行槽位（但 PostgreSQL 中的任务记录不丢）
2. **无认证**: API 无鉴权，仅适合内网或本地使用
3. **粗粒度资源控制**: CPU/内存限制仅用于调度决策，不做 cgroup 级别的硬隔离
