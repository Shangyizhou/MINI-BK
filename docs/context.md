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
| 队列/缓存 | Redis | ✅ |
| 进程间通信 | gRPC + Protobuf | ✅ |
| 一致性存储 | etcd | Phase 4 |
| 前端 | React + Ant Design | Phase 3+ |
| 可观测性 | Prometheus + Grafana | Phase 7 |

## 当前阶段

**Phase 3 — 节点管理与远程执行** (已完成)

在 Phase 2 基础上引入节点发现、心跳保活、gRPC 远程执行和节点选择调度。Server 同时提供 HTTP API 和 gRPC 端点，Agent 通过 gRPC 注册、心跳、拉取任务并报告结果。

### Phase 3 新增特性

- **节点注册与心跳**: Agent 启动时通过 gRPC Register 注册，周期性 Heartbeat 上报资源使用
- **节点离线检测**: NodeManager 每 5 秒检查节点心跳超时（默认 30 秒），超则标记 Offline
- **节点管理 API**: 列表、详情、Drain、Uncordon 操作
- **标签选择调度**: 任务通过 `node_selector` 指定目标节点标签，Scheduler SelectNode 三层筛选
- **gRPC 远程执行**: Agent PullTask 轮询拉取任务，本地执行后 ReportResult 汇报结果
- **节点选择器 (NodeSelector)**: 任务模型新增 `node_selector` JSONB 字段和 `assigned_node_id` 字段
- **Agent 二进制**: `cmd/agent` 独立入口，支持 `-server-addr` 和 `-labels` 参数

## 架构分层

```
cmd/server/          ← 入口：组装所有组件
cmd/agent/           ← Agent 入口：gRPC 客户端
internal/
├── api/             ← HTTP 层：Gin router + handler
├── service/         ← 业务逻辑层
├── scheduler/       ← 调度器：资源感知 ticker 循环 + 节点选择
├── executor/        ← 执行器：os/exec 进程管理
├── model/           ← 数据模型 + 状态机 + 节点模型
├── store/           ← 持久化层：PostgreSQL + Redis
├── queue/           ← 任务队列：Redis 队列 / 内存队列
├── logstream/       ← 实时日志流：SSE + Redis Stream
├── middleware/       ← HTTP 中间件：接口限流
├── nodemanager/     ← 节点管理器：注册、心跳、离线检测、标签匹配（Phase 3）
├── grpcserver/      ← gRPC 服务端：Agent 注册/心跳/PullTask/ReportResult（Phase 3）
├── grpcclient/      ← gRPC 客户端连接池（Phase 3）
└── config/          ← 配置管理：Viper
```

## 数据流

### 本地执行

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

### 远程执行 (PullTask)

```
Agent 启动
  → gRPC Register 注册到 Server
  → 定时 Heartbeat 上报资源
  → 定时 PullTask 轮询任务

Server:
  HTTP POST /api/v1/tasks (with node_selector)
    → task_service.go: CreateTask()
    → scheduler.go: tick() 发现任务
    → SelectNode() 按标签+资源筛选
    → dispatchRemote() → 推入节点队列

Agent:
  PullTask 返回任务 → 本地 executor.Run()
  → ReportResult 汇报结果

Server:
  HandleRemoteResult() → completeTask() / failTask()
```

## 存储职责

| 数据类型 | PostgreSQL | Redis | 说明 |
|----------|------------|-------|------|
| 任务记录 | ✅ 主 | | 所有任务的历史事实 |
| 执行日志 | ✅ | ✅ 实时流 | stdout/stderr 落库 + Redis Stream 实时推送 |
| 任务队列 | | ✅ 主 | 优先使用 Redis List/SortedSet，降级到 InMemQueue |
| 节点记录 | ✅ 主 | | 节点注册信息+资源数据 |
| 幂等去重 | | ✅ | SETNX 5 分钟窗口 |
| 每日统计 | | ✅ | Hash 记录每日提交/成功/失败 |

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

### 节点选择

- 任务可指定 `node_selector` 标签约束
- SelectNode 三层筛选：标签匹配 → 资源过滤 → 最少负载
- 无匹配节点时降级到本地执行

### 错误处理

- 不忽略 error，使用 `fmt.Errorf("context: %w", err)` wrap
- API 层返回统一的 `{"error": "..."}` 格式
- 结构化日志使用 `log/slog`

## 验证

### 快速验证（无外部依赖）

```bash
# 1. 静态检查 + 单元测试 + 编译（无需 PostgreSQL）
go vet ./...
go test ./internal/... -count=1      # Redis 相关测试 SKIP（无 Redis），其余 PASS
go build ./cmd/server                # 编译成功，无输出
go build ./cmd/agent                 # Agent 编译成功，无输出
```

### 完整验证（需要 Docker + PostgreSQL + Redis）

按以下顺序执行，每一步附预期输出。

**Step 1: 启动 PostgreSQL + Redis + 运行迁移**

```bash
./scripts/setup-pg.sh
```

预期输出：
```
=== Setting up PostgreSQL ===
...
PostgreSQL is ready!

=== Setting up Redis ===
...
Redis is ready!
```

**Step 2: 单元测试（含 store 包）**

```bash
go test ./internal/... -v -count=1
```

预期输出：10 个包全部 `ok`（Redis 相关测试不再 SKIP）。

**Step 3: 启动服务**

```bash
go run ./cmd/server
```

预期输出：
```json
{"level":"INFO","msg":"已连接到 PostgreSQL"}
{"level":"INFO","msg":"已连接到 Redis"}
{"level":"INFO","msg":"调度器已启动",...}
{"level":"INFO","msg":"服务启动中","addr":"0.0.0.0:8080"}
```

**Step 4: 手动验证 Phase 1 API（另开终端）**

```bash
# 创建任务
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"hello","command":"echo hello world"}'
# → {"task_uid":"xxx-xxx","status":"pending","created_at":"..."}

# 等 1 秒后查状态
sleep 1
curl -s http://localhost:8080/api/v1/tasks
# → {"tasks":[...],"total":1,"page":1,"size":20}

# 查看日志
curl -s http://localhost:8080/api/v1/tasks/<task_uid>/log
# → {"stdout":"hello world\n","stderr":""}

# 资源余量
curl -s http://localhost:8080/api/v1/resources
# → {"cpu_cores":N,"memory_mb":8192,...}

# 统计
curl -s http://localhost:8080/api/v1/stats
# → {"total_tasks":1,"success":1,"failed":0,"running":0,"success_rate":1}
```

**Step 5: Phase 2 — 幂等性验证**

```bash
# 相同命令和目录提交两次，第二次应被拒绝
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"idempotent","command":"echo same","workdir":"/tmp"}'
# → 201 Created

curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"idempotent","command":"echo same","workdir":"/tmp"}'
# → 409 Conflict 或 500，错误消息含 "duplicate"
```

**Step 6: Phase 2 — 实时日志流 (SSE)**

```bash
# 创建任务，然后连接 SSE 端点
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"sse","command":"echo sse-stream"}'
# → {"task_uid":"xxx-xxx",...}

curl -N http://localhost:8080/api/v1/tasks/xxx-xxx/log/stream
# → data: {"line":"sse-stream","stream":"stdout",...}
# → event: done
```

**Step 7: Phase 2 — 重试验证**

```bash
# 提交会失败的任务，观察自动重试
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"retry","command":"exit 1"}'
# → 201 Created

# 稍后查询，检查 retry_count 字段
curl -s http://localhost:8080/api/v1/tasks/<task_uid>
# → {"retry_count":N,"max_retries":3,"status":"failed",...}
```

**Step 8: Phase 2 — 每日统计**

```bash
curl -s http://localhost:8080/api/v1/stats/daily
# → {"submitted":"1","success":"1","failed":"0"}
```

**Step 9: 取消任务**

```bash
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"long-task","command":"sleep 60"}'
# → {"task_uid":"yyy-yyy",...}

curl -s -X POST http://localhost:8080/api/v1/tasks/yyy-yyy/cancel
# → {"message":"任务已取消"}
```

**Step 10: 超时任务**

```bash
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"timeout-task","command":"sleep 30","timeout_sec":1}'
# 等 2 秒后查状态 → "failed", error_message 含 "timeout"
```

**Step 11: Phase 3 — 节点管理 API**

```bash
# 列出节点
curl -s http://localhost:8080/api/v1/nodes
# → {"nodes":[...]}

# Drain 节点
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/drain
# → {"message":"节点已设为 drain 状态"}

# Uncordon 节点
curl -s -X POST http://localhost:8080/api/v1/nodes/<node_id>/uncordon
# → {"message":"节点已恢复在线"}
```

**Step 12: Phase 3 — 节点选择器任务**

```bash
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"selector","command":"echo on-node","node_selector":{"os":"linux"}}'
# → {"task_uid":"xxx-xxx",...}
```

### 一键开发启动

```bash
./scripts/dev.sh    # PG + 迁移 + 编译 + 启动，一条命令
```

### Docker Compose 启动

```bash
docker-compose -f deployments/docker-compose.yml up
# 启动 PostgreSQL + Redis + Server(:8080, :50051) + Agent
```

### 便捷脚本清单

| 脚本 | 用途 |
|------|------|
| `./scripts/setup-pg.sh` | 创建 PG 容器 + 运行迁移 |
| `./scripts/dev.sh` | 一键开发启动 |
| `./scripts/test.sh` | 单元测试 + go vet |
| `./scripts/test-integration.sh` | 集成测试（需服务已启动） |
| `./scripts/build.sh` | 构建二进制到 `bin/mini-bk-server` |

## 已知风险

1. **无分布式能力**: 当前单机执行，服务重启会丢失内存中的执行槽位（但 PostgreSQL 中的任务记录不丢）
2. **无认证**: API 无鉴权，仅适合内网或本地使用
3. **粗粒度资源控制**: CPU/内存限制仅用于调度决策，不做 cgroup 级别的硬隔离
4. **Agent 单点**: Server 单点故障将导致所有 Agent 失联
5. **拉取延迟**: Agent PullTask 轮询间隔 2 秒，任务分配有秒级延迟
