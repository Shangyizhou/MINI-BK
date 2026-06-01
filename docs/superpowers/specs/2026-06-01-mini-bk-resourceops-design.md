# Mini-BK ResourceOps 设计文档

> **面向 Linux/容器资源的任务调度与运维管控平台**
>
> 版本: v1.0 | 日期: 2026-06-01 | 状态: 已确认

---

## 1. 项目总纲

### 1.1 项目定位

借鉴蓝鲸体系的经典能力（CMDB 管资产、节点管理装 Agent、作业平台做脚本执行/文件分发/定时任务、标准运维做流程编排、监控日志做观测、故障自愈做自动处理），但不是复刻蓝鲸，而是用 Go 生态从零构建一个小型基础设施平台。

**核心价值：** 把 **资源、任务、节点、执行、日志、权限、调度、监控、流程** 串成一个闭环。

**简历定位：**「基于 Go/Redis/etcd/gRPC 的分布式资源调度与作业管控平台」

### 1.2 项目背景

公司内部有一批 Linux 机器、构建机、测试机、转码机、GPU 机器或容器节点，开发、测试、运维、平台人员每天都需要提交任务、执行脚本、分发文件、部署服务、查看日志、观察机器资源、处理失败任务。

早期的痛点：
- 资源不可见（谁在用哪台机器、用了多少资源，完全不知道）
- 任务不可控（SSH 上去跑脚本，跑完没跑完没人知道）
- 执行不可追踪（日志散落在各机器上，出问题找不到）
- 权限不可审计（谁在什么时候做了什么操作，没有记录）
- 失败不可恢复（任务失败了只能手动重跑，没有自动重试和告警）

### 1.3 核心理念（贯穿 12 期）

| 理念 | 含义 |
|------|------|
| **先跑通再分布式** | 一期单机理解本质，逐步引入 Redis/etcd/Agent |
| **PostgreSQL 是历史事实库** | 所有任务、执行、审计的最终记录 |
| **Redis 是高速临时层** | 队列、日志流、限流、缓存、短期状态 |
| **etcd 是控制面一致性状态** | 节点注册、调度锁、Leader Election、服务发现 |
| **每个阶段都可演示** | 每期结束都是一个可跑可测的完整系统 |

### 1.4 技术栈

| 层 | 选择 | 引入阶段 |
|----|------|----------|
| 语言 | Go 1.22+ | 一期 |
| HTTP 框架 | Gin | 一期 |
| 数据库 | PostgreSQL | 一期 |
| 进程管理 | os/exec + goroutine + channel | 一期 |
| 队列/缓存 | Redis（go-redis/v9） | 二期 |
| 进程间通信 | gRPC + Protobuf | 三期 |
| 一致性存储 | etcd（clientv3） | 四期 |
| 前端 | React + Ant Design | 三期后 |
| 部署 | Docker Compose → Kubernetes | 一期/十期 |
| 可观测性 | Prometheus + Grafana | 七期 |

### 1.5 数据存储职责矩阵

| 数据类型 | PostgreSQL | Redis | etcd | 本地文件 | Prometheus |
|----------|------------|-------|------|----------|------------|
| 任务记录（历史） | ✅ 主 | - | - | - | - |
| 执行日志 | ✅ 历史 | ✅ Stream(实时) | - | ✅ 缓冲 | - |
| 节点信息 | ✅ 快照 | ✅ 心跳缓存 | ✅ 注册/存活 | - | - |
| 任务队列 | - | ✅ 主 | - | - | - |
| 调度锁 | - | ✅ 临时 | ✅ 主 | - | - |
| Leader Election | - | - | ✅ | - | - |
| 服务发现 | - | - | ✅ | - | - |
| 配置中心 | - | - | ✅ | - | - |
| 用户/角色/权限 | ✅ 主 | ✅ 缓存 | - | - | - |
| Metrics | - | - | - | - | ✅ |
| 脚本文件 | ✅ 元数据 | - | - | ✅ 文件 | - |
| 发布包 | ✅ 元数据 | - | - | ✅ 文件 | - |

### 1.6 目录结构（渐进式单体）

```
mini-bk/
├── cmd/
│   ├── server/          # API Server 入口（一期）
│   └── agent/           # Agent 入口（三期）
├── internal/
│   ├── api/             # HTTP handler（一期）
│   ├── service/         # 业务逻辑层（一期）
│   ├── scheduler/       # 调度器（一期内存→二期Redis→四期etcd）
│   ├── executor/        # 执行器（一期本地→三期gRPC远程）
│   ├── model/           # 数据模型（一期）
│   ├── store/           # 持久化层 PostgreSQL（一期）
│   ├── queue/           # 队列抽象接口（二期）
│   ├── node/            # 节点管理（三期）
│   ├── script/          # 脚本库管理（五期）
│   ├── filetransfer/    # 文件分发/拉取（五期）
│   ├── cron/            # 定时任务（五期）
│   ├── workflow/        # 流程编排引擎（六期）
│   ├── metrics/         # Prometheus metrics（七期）
│   ├── alert/           # 告警规则+通知（七期）
│   ├── selfheal/        # 故障自愈（八期）
│   ├── deploy/          # 发布/部署（九期）
│   ├── container/       # Docker + K8S 接入（十期）
│   ├── auth/            # 权限与审计（十一期）
│   ├── openapi/         # 开放 API（十二期）
│   └── plugin/          # 插件注册+沙箱（十二期）
├── pkg/
│   ├── proto/           # gRPC proto 定义（三期）
│   └── common/          # 共享工具
├── configs/             # 配置文件
├── migrations/          # 数据库迁移
├── scripts/             # 工具脚本
├── deployments/         # Docker Compose / K8S manifests
├── docs/
│   └── superpowers/
│       └── specs/       # 设计文档
├── go.mod
├── Makefile
└── Dockerfile
```

### 1.7 Module 路径

`github.com/shangyizhou/mini-bk`

---

## 2. 一期 — 单机版资源任务平台

> **阶段目标：** 跑通任务调度最小闭环——提交任务、排队、执行、查日志、看结果。
>
> **引入组件：** Go, Gin, PostgreSQL, goroutine/channel, os/exec

### 2.1 目标与非目标

**目标：**
- 用户创建任务（command, cpu/memory/timeout/workdir/env/priority）
- 系统维护本机资源池，资源够则执行，不够则 Pending
- 任务状态机：Created → Pending → Running → Success/Failed/Canceled
- 任务列表、详情、日志、取消、重跑
- 并发控制（最多同时跑 N 个任务）
- 超时自动 kill
- 执行历史落库

**非目标：**
- 不做分布式（单机执行）
- 不做前端
- 不做脚本库、定时任务
- 不做文件分发
- 不做权限系统

### 2.2 任务数据模型

```sql
CREATE TABLE tasks (
    id            BIGSERIAL PRIMARY KEY,
    task_uid      VARCHAR(36)  NOT NULL UNIQUE,          -- 外部标识 UUID
    name          VARCHAR(255) NOT NULL,                  -- 任务名称
    command       TEXT         NOT NULL,                  -- 要执行的命令
    workdir       VARCHAR(512) DEFAULT '/tmp',            -- 工作目录
    env           JSONB        DEFAULT '{}',              -- 环境变量 {"KEY": "val"}
    cpu_limit     INT          DEFAULT 0,                 -- CPU 核数限制 (0=不限)
    memory_limit  INT          DEFAULT 0,                 -- 内存 MB 限制 (0=不限)
    timeout_sec   INT          DEFAULT 300,               -- 超时秒数
    priority      INT          DEFAULT 0,                 -- 优先级(越大越优先)
    status        VARCHAR(20)  NOT NULL DEFAULT 'created',-- created|pending|running|success|failed|canceled
    exit_code     INT,                                    -- 进程退出码
    stdout        TEXT,                                   -- 标准输出
    stderr        TEXT,                                   -- 标准错误
    error_message TEXT,                                   -- 系统级错误信息
    pid           INT,                                    -- 操作系统进程 ID
    started_at    TIMESTAMPTZ,                              -- 开始执行时间
    finished_at   TIMESTAMPTZ,                              -- 结束时间
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_priority ON tasks(status, priority DESC);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);
```

### 2.3 任务状态机

```
                  ┌──────────┐
                  │ Created  │
                  └────┬─────┘
                       │ Scheduler 检查资源
                       ▼
                  ┌──────────┐
          ┌──────│ Pending  │◄─────── 资源不足,等待下次调度
          │      └────┬─────┘
          │           │ 资源充足,分配执行槽位
          │           ▼
          │      ┌──────────┐
Cancel ◄──┼──────│ Running  │─────── 超时(timeout kill)
          │      └──┬──┬──┘
          │         │  │
          │    正常退出│  │异常退出(exit_code≠0)
          │         │  │
          │         ▼  ▼
          │      ┌──────────┐   ┌──────────┐
          └─────►│ Canceled │   │ Success  │  │ Failed   │
                 └──────────┘   └──────────┘  └──────────┘
```

状态流转规则：
- `Created → Pending`: Scheduler 选取 Created 任务，尝试分配资源
- `Pending → Running`: 资源满足，启动进程
- `Pending → Canceled`: 任务被取消（还没开始执行）
- `Running → Success`: 进程正常退出，exit_code = 0
- `Running → Failed`: 进程异常退出 或 超时被 kill
- `Running → Canceled`: 用户主动取消，发送 SIGTERM → SIGKILL
- 终态：Success, Failed, Canceled（不可再流转）

### 2.4 API 设计

```
POST   /api/v1/tasks                    创建任务
GET    /api/v1/tasks                    任务列表 (?status=&page=&size=)
GET    /api/v1/tasks/:task_uid          任务详情
POST   /api/v1/tasks/:task_uid/cancel   取消任务
POST   /api/v1/tasks/:task_uid/rerun    重跑任务 (生成新 task_uid)
GET    /api/v1/tasks/:task_uid/log      获取任务输出 (stdout+stderr)
GET    /api/v1/resources                本机资源余量
GET    /api/v1/stats                    统计 (今日提交/成功/失败/平均耗时)
```

**POST /api/v1/tasks 请求体：**
```json
{
  "name": "backup-db",
  "command": "pg_dump -U postgres mydb > /backup/mydb.sql",
  "workdir": "/tmp",
  "env": {"PGPASSWORD": "secret"},
  "cpu_limit": 2,
  "memory_limit": 512,
  "timeout_sec": 600,
  "priority": 10
}
```

**POST /api/v1/tasks 响应体：**
```json
{
  "task_uid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "pending",
  "created_at": "2026-06-01T10:00:00Z"
}
```

### 2.5 Scheduler & Executor 设计

```
┌──────────────────────────────────────┐
│           Scheduler (goroutine)       │
│                                      │
│  ticker: 每 500ms 检查一次            │
│  1. SELECT Created 任务 (ORDER BY     │
│     priority DESC, created_at ASC)   │
│  2. 计算本机资源余量                  │
│  3. 资源够 → 创建 Executor           │
│  4. 资源不够 → 标记 Pending          │
│  5. 检查 Pending 任务是否可以调度    │
│  6. 检查超时 Running 任务            │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│        Executor (per-task goroutine)  │
│                                      │
│  ctx, cancel := context.WithTimeout  │
│  cmd := exec.CommandContext(ctx, ...) │
│  cmd.Wait()                          │
│  → 更新 status, exit_code, stdout    │
│  → 释放资源槽位                      │
└──────────────────────────────────────┘
```

**并发控制：** 用 buffered channel 做信号量
```go
execSlots := make(chan struct{}, maxConcurrent) // 如 N=10
// 执行前: execSlots <- struct{}{}
// 执行后: <-execSlots
```

**资源计算：**
```go
// 本机总资源 - 所有 Running 任务已占用资源 = 可用资源
availableCPU := totalCPU - allocatedCPU  // allocatedCPU = sum(running_tasks.cpu_limit)
availableMem := totalMem - allocatedMem
```

### 2.6 一期目录结构

```
mini-bk/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── router.go
│   │   ├── task_handler.go
│   │   └── resource_handler.go
│   ├── service/
│   │   └── task_service.go
│   ├── scheduler/
│   │   └── scheduler.go
│   ├── executor/
│   │   └── executor.go
│   ├── model/
│   │   └── task.go
│   └── store/
│       ├── postgres.go
│       └── task_store.go
├── configs/
│   └── config.yaml
├── migrations/
│   └── 001_create_tasks.sql
├── go.mod
├── Makefile
└── Dockerfile
```

### 2.7 验收标准

| # | 标准 | 验证方式 |
|---|------|----------|
| 1 | 创建任务后可在 PostgreSQL 查到状态 Created | 集成测试 |
| 2 | Pending 任务在资源足够时自动变为 Running | 集成测试 |
| 3 | 简单命令 `echo hello` 执行成功，状态 Success，stdout="hello\n" | 集成测试 |
| 4 | 超时任务自动 kill，状态 Failed，error_message 含 "timeout" | 集成测试 |
| 5 | 取消 Running 任务，进程被杀，状态 Canceled | 集成测试 |
| 6 | 并发达到上限时新任务进入 Pending | 集成测试 |
| 7 | `GET /api/v1/tasks` 返回分页列表 | 集成测试 |
| 8 | `GET /api/v1/tasks/:uid/log` 返回 stdout/stderr | 集成测试 |
| 9 | 服务重启后，之前的历史任务仍可查询 | 集成测试 |
| 10 | 资源余量 API 返回正确的 CPU/内存可用量 | 集成测试 |

### 2.8 测试策略

- **单元测试：** Task 状态机、Scheduler 调度逻辑（mock store）
- **集成测试：** 全链路 `创建任务 → 等待完成 → 验证结果`，用真实 PostgreSQL（testcontainers-go）
- **一期不做：** 前端测试、性能测试、压力测试

---

## 3. 二期 — Redis 队列版任务平台

> **阶段目标：** 任务提交与消费解耦，支持服务重启不丢任务，API Server 可横向扩展。
>
> **新增组件：** Redis（go-redis/v9）

### 3.1 目标与非目标

**目标：**
- Redis 作为任务队列中间层
- 支持普通队列、优先级队列、延迟队列
- 任务幂等，防止重复执行
- 失败重试次数、重试间隔、最大运行时长
- 任务日志实时写 Redis Stream，前端 SSE 看实时日志
- 任务取消通过 Redis Pub/Sub 发送 cancel 信号
- 接口限流（单用户每分钟 N 次）
- 任务统计看板（今日提交数、成功率、平均耗时、排队时长）

**非目标：**
- 不做分布式执行（仍在 API Server 本机执行）

### 3.2 队列抽象层

```go
// internal/queue/queue.go
type TaskQueue interface {
    Push(ctx context.Context, task *Task) error
    Pop(ctx context.Context) (*Task, error)            // 阻塞式
    PushPriority(ctx context.Context, task *Task) error  // 优先级队列
    PushDelayed(ctx context.Context, task *Task, delay time.Duration) error
    Ack(ctx context.Context, taskUID string) error      // 确认消费
}
```

两种实现：
- `internal/queue/inmem.go` — channel 实现（测试/fallback）
- `internal/queue/redis.go` — Redis 实现（生产）

### 3.3 Redis 职责清单

| 场景 | Redis 数据结构 |
|------|---------------|
| 普通任务队列 | `tasks:queue:pending` — List (BLPOP) |
| 优先级队列 | `tasks:queue:priority` — ZSET (score=priority) |
| 延迟队列 | `tasks:queue:delayed` — ZSET (score=执行时间戳) |
| 任务幂等去重 | `tasks:dedup:<task_hash>` — SETEX (TTL=窗口) |
| 实时日志流 | `tasks:log:<task_uid>` — Stream (XADD/XREAD) |
| 接口限流 | `ratelimit:<user_id>:<window>` — INCR + EXPIRE |
| 任务取消信号 | `tasks:cancel:<task_uid>` — Pub/Sub 频道 |
| 实时统计缓存 | `stats:daily:<date>` — Hash (HINCRBY) |

### 3.4 新增接口

```
GET  /api/v1/tasks/stats/daily              今日统计
GET  /api/v1/tasks/:task_uid/log/stream     SSE 实时日志流
```

### 3.5 关键设计决策

- **先落库再入队：** 任务先写 PostgreSQL（持久化），再 Push Redis 队列
- **双写策略：** Worker 从 Redis Pop 任务后，更新 PostgreSQL 状态为 Running
- **幂等：** 对 (command + workdir + env + 5min 时间窗口) 取 hash，相同 hash 在窗口内拒绝重复提交
- **限流：** 基于用户 ID + 分钟窗口，Redis INCR 计数器，达到阈值返回 429

### 3.6 验收标准

| # | 标准 |
|---|------|
| 1 | 服务重启后 Redis 中的 Pending 任务不丢失 |
| 2 | 相同任务在 5 分钟内重复提交被拒绝（幂等） |
| 3 | 实时日志 SSE 推送延迟 < 1s |
| 4 | 单用户每分钟超过 100 次提交返回 429 |
| 5 | 延迟任务在指定时间后才被消费 |
| 6 | 任务失败后按配置自动重试（次数+间隔） |

---

## 4. 三期 — Agent 分布式执行平台

> **阶段目标：** 任务在多台机器上执行，Server 根据节点资源智能调度。
>
> **新增组件：** gRPC, Protobuf, Agent 二进制

### 4.1 目标与非目标

**目标：**
- Agent 启动注册节点，周期上报 CPU/内存/磁盘/负载/GPU
- Server 展示节点在线/离线/异常
- Scheduler 根据节点资源选择目标 Agent
- Server 通过 gRPC 下发任务给 Agent
- Agent 执行命令、管理进程、采集日志、上报结果
- Agent 支持任务取消、超时 kill、工作目录隔离
- 节点标签（linux, gpu, ssd, build, test, prod）
- 任务可指定标签选择器
- 节点 Drain / Cordon / Uncordon
- Agent 版本管理和心跳超时判定

**非目标：**
- 不做 etcd（节点状态先存 PostgreSQL + Redis 缓存）
- 不做前端（纯 API）

### 4.2 架构

```
┌─────────────────────┐     gRPC      ┌─────────────────────┐
│   API Server         │◄────────────►│   Agent (节点1)      │
│   ┌───────────────┐ │               │   - 心跳上报          │
│   │ Scheduler     │ │               │   - 资源采集          │
│   │ (资源感知调度)  │ │               │   - 命令执行          │
│   │ NodeManager   │ │               │   - 日志回传          │
│   │ AgentRegistry │ │               │   - 文件分发          │
│   └───────────────┘ │               └─────────────────────┘
└──────────┬──────────┘
           │                         ┌─────────────────────┐
           │ gRPC                    │   Agent (节点2)      │
           └────────────────────────►│   ...               │
                                     └─────────────────────┘
```

### 4.3 gRPC Proto 定义

```protobuf
service AgentService {
    // Agent → Server: 注册
    rpc Register(RegisterRequest) returns (RegisterResponse);
    // Agent → Server: 心跳(含资源上报)
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
    // Server → Agent: 下发任务
    rpc SubmitTask(TaskRequest) returns (TaskResponse);
    // Server → Agent: 取消任务
    rpc CancelTask(CancelRequest) returns (CancelResponse);
    // Agent → Server: 日志流(双向流)
    rpc StreamLog(stream LogChunk) returns (stream ControlMessage);
    // Agent → Server: 任务结果上报
    rpc ReportResult(ResultRequest) returns (ResultResponse);
}
```

### 4.4 调度算法（三层过滤）

```go
func (s *Scheduler) SelectNode(task *Task) (*Node, error) {
    // Layer 1: 标签匹配
    candidates := s.nodeMgr.FindByLabels(task.NodeSelector)
    // Layer 2: 资源过滤
    candidates = filterByResources(candidates, task.CPU, task.Memory)
    // Layer 3: 打分排序 (LeastAllocated 优先)
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].AllocatedRatio() < candidates[j].AllocatedRatio()
    })
    if len(candidates) == 0 { return nil, ErrNoNodeAvailable }
    return candidates[0], nil
}
```

### 4.5 节点管理

| 功能 | 说明 |
|------|------|
| 节点注册 | Agent 启动时向 Server 注册（hostname, ip, labels, total resources） |
| 心跳 | 每 10s 上报资源快照，Server 30s 未收到标记 OFFLINE |
| 节点标签 | `linux`, `gpu`, `ssd`, `build`, `test`, `prod`，支持自定义 |
| Drain | 禁止调度新任务，已运行任务不受影响 |
| Cordon/Uncordon | 禁止所有调度 / 恢复 |
| Agent 版本 | 心跳时带版本号，Server 可发现版本落后 |

### 4.6 关键设计决策

- Agent 无状态：重启后重新注册，Server 清理旧连接
- 任务超时由 Agent 本地控制（context.WithTimeout），不依赖 Server 持续通信
- 日志流使用 gRPC 双向流，Agent 推送日志块到 Server，Server 可发送 cancel/control 信号
- Agent 执行目录隔离：每个任务在 `<workdir>/tasks/<task_uid>/` 下执行

### 4.7 验收标准

| # | 标准 |
|---|------|
| 1 | Agent 注册后 Server 节点列表可见状态 ONLINE |
| 2 | 任务可根据标签选择器调度到匹配节点 |
| 3 | Agent 心跳超时 30s 后标记 OFFLINE |
| 4 | 任务取消信号在 5s 内到达 Agent 并 kill 进程 |
| 5 | Server 重启后能恢复与已知 Agent 的连接 |
| 6 | 节点 Drain 后新任务不调度到该节点 |

---

## 5. 四期 — etcd 集群状态与调度高可用

> **阶段目标：** 消除 Scheduler 单点，实现多 Scheduler 高可用调度。
>
> **新增组件：** etcd（clientv3）

### 5.1 目标与非目标

**目标：**
- Agent 节点注册到 etcd，使用 lease 实现节点存活
- Scheduler 监听 etcd 节点变化
- 任务分配结果写入 etcd，CAS 防止重复抢占
- 多个 Scheduler 通过 etcd 做 Leader Election
- 节点状态、服务发现、调度配置存 etcd
- API Server 可从 etcd 获取实时集群视图

**非目标：**
- 不做 etcd 集群运维自动化（手动部署或用 Docker Compose）

### 5.2 etcd 职责清单

| 场景 | etcd 机制 | Key 模式 |
|------|-----------|----------|
| 节点注册+存活 | Lease + KeepAlive | `/nodes/<node_id>/status` |
| Leader Election | Campaign/Observe | `/election/scheduler/` |
| 任务分配防重复 | 事务 CAS | `/tasks/claimed/<task_uid>` |
| 服务发现 | Watch + Get | `/services/<name>/` |
| 调度配置 | Watch | `/config/scheduler/` |

### 5.3 多 Scheduler 架构

```
                 ┌─────────────┐
                 │    etcd      │
                 │  (3节点集群)  │
                 └──┬───┬───┬──┘
                    │   │   │
        ┌───────────┘   │   └───────────┐
        ▼               ▼               ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│Scheduler-1   │ │Scheduler-2   │ │Scheduler-3   │
│(Leader)      │ │(Follower)    │ │(Follower)    │
│ 实际调度任务  │ │ 待命         │ │ 待命         │
└──────────────┘ └──────────────┘ └──────────────┘
```

只有 Leader 执行调度逻辑。Leader 崩溃后 etcd Session 过期，自动重新选主。

### 5.4 防重复调度（CAS）

```go
func (s *Scheduler) ClaimTask(taskUID string) (bool, error) {
    key := fmt.Sprintf("/tasks/claimed/%s", taskUID)
    txn := s.etcd.Txn(ctx).
        If(clientv3.Compare(clientv3.Version(key), "=", 0)). // key 不存在
        Then(clientv3.OpPut(key, s.schedulerID)).            // 写入自己的 ID
        Else(clientv3.OpGet(key))                             // 已被别人抢占
    resp, _ := txn.Commit()
    return resp.Succeeded, nil // true=抢占成功, false=已被抢占
}
```

### 5.5 数据架构总结（四期结束）

```
┌──────────────┬─────────────────┬─────────────────┬─────────────────┐
│   数据层      │   PostgreSQL    │     Redis       │      etcd       │
├──────────────┼─────────────────┼─────────────────┼─────────────────┤
│   角色       │ 历史事实库       │ 高速临时层       │ 控制面一致性     │
│   数据特征   │ 写入后不改       │ 高吞吐短生命周期  │ 强一致小数据量   │
│   典型数据   │ 任务记录/审计    │ 队列/日志流/缓存  │ 节点/锁/选举    │
│   查询方式   │ SQL             │ 数据结构操作     │ Watch+Get      │
│   数据丢失   │ 不可接受         │ 可容忍(重建)     │ 不可接受         │
└──────────────┴─────────────────┴─────────────────┴─────────────────┘
```

---

## 6. 五~十二期路线图

### 6.1 五期 — 作业平台能力增强

| 维度 | 内容 |
|------|------|
| **目标** | 脚本库管理 + 文件分发/拉取 + 批量节点执行 + 定时任务 + 高危命令拦截 |
| **非目标** | 不做 web SSH/terminal、不做脚本在线编辑器 |
| **模块路径** | `internal/script/`、`internal/filetransfer/`、`internal/cron/` |
| **关键决策** | 脚本存储：元数据 PostgreSQL + 文件本地/Object Storage；文件分发基于 gRPC 流式传输；高危命令用正则黑名单拦截；定时任务复用 Cron 表达式 + 普通任务执行链路 |
| **预估工期** | 实际 3-4 周 |

### 6.2 六期 — 流程编排 / 标准运维

| 维度 | 内容 |
|------|------|
| **目标** | DAG 流程引擎，串行/并行/条件分支/人工确认/失败跳转/变量传递 |
| **非目标** | 不做可视化拖拽编排页面（先做 API + JSON 模板）、不做 BPMN 兼容 |
| **模块路径** | `internal/workflow/`（dag.go, executor.go, variable.go） |
| **关键决策** | DAG 用邻接表存储（JSONB 存 DAG 结构），每个节点是统一接口；节点类型：script/file/http/wait/approval/notification；流程状态机：created → running → paused → resumed → success/failed/canceled |

**DAG 模板示例（JSON）：**
```json
{
  "name": "服务发布流程",
  "nodes": [
    {"id": "stop",    "type": "script", "command": "systemctl stop myapp"},
    {"id": "deploy",  "type": "script", "command": "cp /tmp/myapp /opt/myapp"},
    {"id": "health",  "type": "http",   "url": "http://localhost:8080/health", "retry": 3},
    {"id": "start",   "type": "script", "command": "systemctl start myapp"},
    {"id": "approve", "type": "approval", "message": "切流审批"}
  ],
  "edges": [
    {"from": "stop",    "to": "deploy"},
    {"from": "deploy",  "to": "health"},
    {"from": "health",  "to": "start"},
    {"from": "start",   "to": "approve", "condition": "env == 'prod'"}
  ]
}
```

| **预估工期** | 实际 4-5 周 |

### 6.3 七期 — 监控、日志与告警

| 维度 | 内容 |
|------|------|
| **目标** | Prometheus metrics + Grafana Dashboard + 日志按 trace_id 检索 + 多通道告警通知 |
| **非目标** | 不替换专业 APM、不做全链路追踪（先做任务级 trace） |
| **模块路径** | `internal/metrics/`、`internal/alert/` |
| **关键决策** | 告警规则先硬编码再迁移到配置驱动；通知通道：邮件 SMTP + 企业微信 Webhook + 飞书 Webhook；Trace ID 在 API 入口生成，透传至 Agent 执行 |
| **预估工期** | 实际 2-3 周 |

### 6.4 八期 — 故障自愈与自动化处理

| 维度 | 内容 |
|------|------|
| **目标** | 告警→策略→作业→反馈闭环，常见故障自动处理 |
| **非目标** | 不做 ML 驱动的异常检测、不做复杂的根因分析 |
| **模块路径** | `internal/selfheal/`（strategy.go, action.go, audit.go） |
| **关键决策** | 自愈策略 = 触发器 + 条件 + 动作 + 是否需要审批；动作执行复用作业平台和流程编排能力；自愈失败自动升级为人工工单 |

**自愈策略示例：**
```json
{
  "name": "磁盘满自动清理",
  "trigger": "node.disk_usage > 90%",
  "cooldown": "30m",
  "action": {"type": "script", "command": "find /var/log -name '*.log' -mtime +7 -delete"},
  "require_approval": false,
  "escalate_on_failure": true
}
```

| **预估工期** | 实际 2-3 周 |

### 6.5 九期 — 服务发布与变更管控

| 维度 | 内容 |
|------|------|
| **目标** | 应用注册 → 版本包管理 → 灰度/分批发布 → 健康检查 → 失败回滚 → 变更审计 |
| **非目标** | 不做完整的 CI 流水线、不做容器镜像构建 |
| **模块路径** | `internal/deploy/`（app.go, release.go, strategy.go, rollback.go） |
| **关键决策** | 发布策略：普通(全量) / 灰度(百分比) / 分批(批次大小)；健康检查：HTTP 探活 + 进程存活 + 自定义脚本；回滚保留最近 N 个版本的制品；发布窗口/发布冻结 = 配置日历规则 |
| **预估工期** | 实际 3-4 周 |

### 6.6 十期 — 容器与 Kubernetes 接入

| 维度 | 内容 |
|------|------|
| **目标** | Docker 容器管理 + K8S 集群注册/Job 执行/资源查看/日志查询 |
| **非目标** | 不做 K8S 集群生命周期管理、不做 Service Mesh |
| **模块路径** | `internal/container/`（docker.go, k8s.go） |
| **关键决策** | Docker 通过 docker socket/API 交互；K8S 通过 client-go；K8S Job 作为任务执行载体（task → Job, task_uid → job name） |
| **预估工期** | 实际 3-4 周 |

### 6.7 十一期 — 权限中心、审计与多租户

| 维度 | 内容 |
|------|------|
| **目标** | RBAC + 业务空间 + 资源级权限 + 操作审计 + 审批流 + 租户配额 |
| **非目标** | 不做 OAuth2/OIDC 服务端（可集成外部 IDP）、不做细粒度数据行级权限 |
| **模块路径** | `internal/auth/`（rbac.go, audit.go, quota.go, approval.go） |
| **关键决策** | 权限模型：User → Role → Permission；业务空间 = 一组节点的资源池 + 关联用户/角色；操作审计异步写入；API Token = JWT + 过期 + 权限范围限制 |
| **预估工期** | 实际 3-4 周 |

### 6.8 十二期 — 开放平台与插件生态

| 维度 | 内容 |
|------|------|
| **目标** | OpenAPI 文档 + Webhook + 插件注册机制 + 插件沙箱 + SDK/CLI |
| **非目标** | 不做完整的插件市场运营系统、不做计费和结算 |
| **模块路径** | `internal/openapi/`、`internal/plugin/`（registry.go, sandbox.go）、`pkg/mini-bk-cli/` |
| **关键决策** | 插件接口标准化；沙箱优先用进程隔离（独立子进程）；CLI 工具用 Cobra；Terraform Provider 作为远期 stretch goal |
| **预估工期** | 实际 3-4 周 |

---

## 7. 技术栈引入时间线

```
Phase 1    Phase 2    Phase 3    Phase 4    Phase 5    Phase 6    Phase 7     Phase 8-12
  │          │          │          │          │          │          │            │
  Go         │          │          │          │          │          │            │
  Gin        │          │          │          │          │          │            │
  PG         │          │          │          │          │          │            │
  os/exec    │          │          │          │          │          │            │
  chan       │          │          │          │          │          │            │
             Redis      │          │          │          │          │            │
             Stream     │          │          │          │          │            │
                        gRPC       │          │          │          │            │
                        Agent      │          │          │          │            │
                        Protobuf   │          │          │          │            │
                                   etcd       │          │          │            │
                                   Leader     │          │          │            │
                                   Election   │          │          │            │
                                              Cron       │          │            │
                                              Exp        │          │            │
                                              File       │          │            │
                                              Transfer   │          │            │
                                                         DAG        │            │
                                                         Engine     │            │
                                                                    Prometheus  │
                                                                    Grafana     │
                                                                    Alertmanager│
                                                                               Docker SDK
                                                                               client-go
                                                                               JWT/RBAC
                                                                               OpenAPI
```

---

## 8. 全局约束与约定

### 8.1 代码规范

- Go 代码遵循官方 Code Review Comments 和 `gofmt` 格式化
- 所有公开函数/类型必须有注释
- 错误处理：不忽略 error，使用 `fmt.Errorf("context: %w", err)` wrap
- 日志：使用 `log/slog`（Go 1.21+ 标准库结构化日志）
- 配置：使用 Viper，支持 YAML 文件 + 环境变量覆盖
- 迁移：使用 golang-migrate 管理 PostgreSQL schema

### 8.2 设计原则

- **接口抽象：** 关键组件（Queue、Store、Executor）都定义接口，新实现可替换
- **先落库再异步：** 任务/审计等关键数据先写 PostgreSQL，再发 Redis/消息
- **Agent 无状态：** 所有状态由 Server 侧管理，Agent 只负责执行和上报
- **向后兼容预留：** 数据模型中预留扩展字段（JSONB），API 路径用 `/v1/` 前缀

### 8.3 每期 spec 模板

后续每期开工前写独立的 spec 文件，模板包含：
1. 背景与目标
2. 非目标（明确不做的事）
3. 模块边界（代码路径、依赖组件）
4. 数据模型（DDL / 关键索引）
5. API / gRPC 接口定义
6. 状态机（如涉及）
7. 关键设计决策
8. 验收标准（可测试的 checklist）
9. 测试用例列表
10. Setup/teardown（migration、配置变更）
11. 下一期预留点

---

## 9. 未决定事项

以下事项在各期开工前再确定：
- 前端具体组件布局（三期后统一规划）
- Grafana Dashboard 的具体面板设计（七期）
- 审批流的具体交互和通知模板（六期/八期/十一期）
- 插件沙箱的安全隔离级别（十二期）
- Terraform Provider 的 API 覆盖范围（十二期 stretch goal）

---

## 10. 附录：决策记录

| 决策 | 结论 | 日期 |
|------|------|------|
| Module 路径 | `github.com/shangyizhou/mini-bk` | 2026-06-01 |
| 数据库 | PostgreSQL | 2026-06-01 |
| Detail 策略 | 一期全详，二~四期中详，五~十二期路线图 | 2026-06-01 |
| 目录结构 | 渐进式单体 | 2026-06-01 |
| 前端引入时机 | 三期后 | 2026-06-01 |
| HTTP 框架 | Gin | 2026-06-01 |
| 演进方案 | 方案 A：渐进式演进 (1→12 期顺序推进) | 2026-06-01 |
