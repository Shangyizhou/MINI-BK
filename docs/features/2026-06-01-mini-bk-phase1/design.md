# Mini-BK Phase 1 Design

## 概述

渐进式单体 Go 应用。分层架构：Gin HTTP → Service → Scheduler + Executor + Store(PostgreSQL)。并发控制用 buffered channel 信号量，超时用 context.WithTimeout，调度用 ticker 轮询。

## 总体方案

> - **目标架构**: 单体 Go 应用，Gin 对外提供 REST API，内部按职责分层
> - **调度策略**: ticker 轮询 + 资源感知，Pending 优先于 Created
> - **持久化**: PostgreSQL 作为历史事实库，所有任务状态变更必须落库
> - **进程管理**: os/exec + sh -c，每任务一个 goroutine 异步执行

## 模块职责

### 配置管理 (config)
- **职责**: 加载 YAML 配置 + MINIBK_ 前缀环境变量覆盖
- **约束**: Viper AutomaticEnv + Unmarshal 有已知限制，关键字段（Server.Port, Database.Host 等）手动补充覆盖
- **文件**: `internal/config/config.go`

### 任务模型 (model)
- **职责**: Task 结构体定义、6 状态状态机、流转合法性校验
- **约束**: 终态（Success/Failed/Canceled）不可逆，取消仅限非终态
- **关键接口**: `NewTask()`, `TransitionTo(target) error`, `IsTerminal() bool`
- **文件**: `internal/model/task.go`

### 持久化 (store)
- **职责**: PostgreSQL CRUD + 按状态查询 + 分页列表
- **约束**: env 字段为 JSONB，使用 json.Marshal/Unmarshal；可空字段使用指针类型
- **关键接口**: `Create()`, `Update()`, `GetByUID()`, `List(status, page, size)`, `GetCreatedTasks()`, `GetPendingTasks()`, `GetRunningTasks()`
- **文件**: `internal/store/postgres.go`, `internal/store/task_store.go`

### 执行器 (executor)
- **职责**: os/exec 进程管理，超时（context.WithTimeout）和取消（context cancel），stdout/stderr 捕获
- **约束**: 统一使用 `sh -c` 执行；继承系统环境变量 + 注入 task.Env；buffered channel 信号量控制并发数
- **关键类型**: `TaskResult{ExitCode, Stdout, Stderr, TimedOut, Error}`
- **文件**: `internal/executor/executor.go`

### 调度器 (scheduler)
- **职责**: ticker 轮询（默认 500ms），资源感知调度，Running 任务超时检测
- **约束**: 单机模式下天然单 Scheduler（Phase 4 才引入 Leader Election）
- **调度流程**: 获取 Running → 计算可用资源 → 调度 Pending → 处理 Created → 检查超时
- **关键接口**: `Start(ctx)`, `GetTotalResources() (cpu, memMB int)`
- **文件**: `internal/scheduler/scheduler.go`

### 业务逻辑 (service)
- **职责**: CreateTask（参数校验+默认值）、CancelTask（状态校验）、RerunTask（复制原任务）
- **约束**: 纯业务逻辑，不直接操作 DB，通过 TaskStore 接口访问
- **文件**: `internal/service/task_service.go`

### HTTP API (api)
- **职责**: Gin router + 7 个 handler，JSON 请求/响应
- **约束**: 统一错误格式 `{"error": "..."}`；路由前缀 `/api/v1/`
- **文件**: `internal/api/router.go`, `internal/api/task_handler.go`, `internal/api/resource_handler.go`

## 数据模型

**tasks 表核心字段:**

| 字段 | 类型 | 说明 |
|------|------|------|
| task_uid | VARCHAR(36) UNIQUE | UUID v4，外部唯一标识 |
| name | VARCHAR(255) | 任务名称（必填） |
| command | TEXT | 执行的命令（必填） |
| workdir | VARCHAR(512) | 工作目录（默认 /tmp） |
| env | JSONB | 环境变量（默认 {}） |
| cpu_limit | INT | CPU 核数限制（0=不限） |
| memory_limit | INT | 内存 MB 限制（0=不限） |
| timeout_sec | INT | 超时秒数（默认 300） |
| priority | INT | 优先级，越大越优先（默认 0） |
| status | VARCHAR(20) | created/pending/running/success/failed/canceled |
| exit_code | INT | 进程退出码（可空） |
| stdout / stderr | TEXT | 标准输出/错误 |
| error_message | TEXT | 系统级错误信息 |
| pid | INT | OS 进程 ID（可空） |
| started_at / finished_at | TIMESTAMPTZ | 执行起止时间（可空） |

索引: `idx_tasks_status`, `idx_tasks_priority` (status, priority DESC), `idx_tasks_created_at`

## 状态机

```
Created ──→ Pending ──→ Running ──→ Success
  │           │            │
  └──→ Canceled ←─────────┘
                         Failed (含超时)
```

- Created → Pending: 资源不足时 Scheduler 标记
- Pending → Running: 资源满足，dispatch
- Pending → Canceled: 用户取消（未开始执行）
- Running → Success: exit_code = 0
- Running → Failed: exit_code ≠ 0 或超时
- Running → Canceled: 用户取消，发送 SIGTERM → SIGKILL
- 终态（Success, Failed, Canceled）不可再流转

## API 设计

```
POST   /api/v1/tasks               创建任务
GET    /api/v1/tasks               任务列表 (?status=&page=&size=)
GET    /api/v1/tasks/:task_uid     任务详情
POST   /api/v1/tasks/:task_uid/cancel  取消任务
POST   /api/v1/tasks/:task_uid/rerun   重跑任务（生成新 task_uid）
GET    /api/v1/tasks/:task_uid/log     获取输出 (stdout+stderr)
GET    /api/v1/resources           本机资源余量
GET    /api/v1/stats               统计（提交数/成功数/失败数/成功率）
```

**POST /api/v1/tasks 请求体:**
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

## 并发模型

```go
execSlots := make(chan struct{}, maxConcurrent) // 默认 10
// 执行前: execSlots <- struct{}{}  (阻塞等待槽位)
// 执行后: <-execSlots              (释放槽位)
```

执行流程：Scheduler.tick() → dispatch(task) → goroutine → executor.Run() → completeTask/failTask → store.Update()

## 测试策略

- **单元测试**: Task 状态机、Scheduler 调度逻辑（mock store/executor）、Config 加载
- **集成测试**: 全链路 创建→等待→验证，需要真实 PostgreSQL（testcontainers-go 或本地 PG）
- **不做**: 前端测试、性能测试、压力测试
