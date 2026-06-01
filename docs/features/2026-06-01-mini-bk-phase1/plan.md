# Mini-BK Phase 1 Implementation Plan

> 详细分步实现（含完整代码和 TDD 步骤）见 [`docs/superpowers/plans/2026-06-01-mini-bk-phase1-implementation.md`](../../superpowers/plans/2026-06-01-mini-bk-phase1-implementation.md)

## 实现顺序

任务按依赖关系排列：底层模块在前，上层组装在后。每个任务遵循 TDD（先写测试确认失败，再实现确认通过）。

---

### 1. 项目骨架初始化 ✅

**文件**: `go.mod`, `Makefile`, 所有目录

- 初始化 module `github.com/shangyizhou/mini-bk`
- 创建目录：`cmd/server`, `internal/{api,service,scheduler,executor,model,store,config}`, `configs`, `migrations`, `scripts`, `deployments`
- Makefile 包含 build/run/test/lint/clean/migrate 目标
- 安装依赖: Gin, lib/pq, Viper, golang-migrate, uuid
- **提交**: `chore: 初始化项目骨架`

---

### 2. 配置管理 ✅

**文件**: `internal/config/config.go`, `configs/config.yaml`

- Config 结构体：Server, Database, Scheduler, Executor 四个子配置
- 使用 Viper 加载 YAML，支持 `MINIBK_` 前缀环境变量覆盖所有字段
- `DatabaseConfig.DSN()` 返回 PostgreSQL 连接字符串
- `ServerConfig.Addr()` 返回监听地址
- **提交**: `feat: 添加配置管理模块`

---

### 3. Task 模型与状态机 ✅

**文件**: `internal/model/task.go`

- `TaskStatus` 类型：created, pending, running, success, failed, canceled
- `Task` 结构体：完整映射 tasks 表所有字段
- `NewTask(name, command)`：生成 UUID，设置默认值
- `TransitionTo(target)`：校验状态流转合法性
- `IsTerminal()`：判断是否为终态
- `ErrTaskNotFound` sentinel error
- **测试**: 13 种流转组合（合法+非法）、终态判断、默认值验证
- **提交**: `feat: 添加任务模型与状态机`

---

### 4. 数据库迁移与连接 ✅

**文件**: `migrations/000001_create_tasks.{up,down}.sql`, `internal/store/postgres.go`

- up 迁移：创建 tasks 表（17 个字段）+ 3 个索引
- down 迁移：DROP TABLE tasks
- `Postgres` 结构体：封装 `*sql.DB`，连接池配置（MaxOpenConns=25, MaxIdleConns=5）
- `NewPostgres(ctx, dsn)`：创建连接并 ping 验证
- **提交**: `feat: 添加 PostgreSQL 连接和迁移脚本`

---

### 5. TaskStore 持久化层 ✅

**文件**: `internal/store/task_store.go`

- `Create(ctx, task)`: INSERT + RETURNING id，JSONB env 字段 marshal
- `Update(ctx, task)`: 更新所有字段（17 个 SET），使用 task_uid 定位
- `GetByUID(ctx, uid)`: SELECT 单条
- `List(ctx, status, page, size)`: COUNT + 分页 SELECT，ORDER BY priority DESC, created_at ASC
- `GetCreatedTasks(ctx)`, `GetPendingTasks(ctx)`, `GetRunningTasks(ctx)`: 按状态查询
- 私有 helper: `scanTask()` 处理可空字段（sql.NullInt64/NullString/NullTime）
- **测试**: Create/Read, List 分页, 按状态筛选, Update 状态, GetPending/Running
- **提交**: `feat: 添加任务持久化层`

---

### 6. Executor 进程执行器 ✅

**文件**: `internal/executor/executor.go`

- `TaskResult` 结构体：ExitCode, Stdout, Stderr, TimedOut, Error
- `NewExecutor(maxConcurrent)`: 创建 buffered channel 信号量
- `Run(ctx, task) *TaskResult`: 获取槽位 → context.WithTimeout → exec.CommandContext("sh", "-c", cmd) → 捕获 stdout/stderr → 释放槽位
- 超时检测：`execCtx.Err() == context.DeadlineExceeded`
- 取消检测：`ctx.Err() == context.Canceled`
- 非零退出码：`exec.ExitError` → `syscall.WaitStatus.ExitStatus()`
- **测试**: 成功执行, 超时, 非零退出, 环境变量, 取消, 并发限制
- **提交**: `feat: 添加进程执行器`

---

### 7. Scheduler 调度器 ✅

**文件**: `internal/scheduler/scheduler.go`

- `TaskStore` 接口：GetCreatedTasks, GetPendingTasks, GetRunningTasks, Update
- `TaskExecutor` 接口：Run(ctx, task) *TaskResult
- `NewScheduler(store, executor, tickInterval, maxConcurrent)`: 自动检测 `runtime.NumCPU()`
- `Start(ctx)`: ticker 循环，ctx.Done() 退出
- `tick(ctx)` 调度逻辑：
  1. 获取 Running 任务 → 计算已分配 CPU/内存
  2. 调度 Pending 任务（如果资源满足）
  3. 处理 Created 任务（资源够 → dispatch，不够 → Pending）
  4. 检查 Running 超时（started_at + timeout_sec < now → failTask）
- `dispatch(ctx, task)`: TransitionTo(Running) → Update → goroutine 执行
- `completeTask/failTask`: 设置终态 + stdout/stderr/finishedAt
- `GetTotalResources()`: 供 API 查询
- **测试**: Created 调度, 资源不足→Pending, Start/Stop 生命周期
- **提交**: `feat: 添加调度器`

---

### 8. TaskService 业务逻辑层 ✅

**文件**: `internal/service/task_service.go`

- `CreateTaskRequest` 结构体：Name(binding:required), Command(binding:required), 可选字段
- `CreateTask(ctx, req)`: NewTask + 覆盖可选值 + store.Create
- `GetTask(ctx, uid)`: 代理 store.GetByUID
- `ListTasks(ctx, status, page, size)`: 校验 page/size → store.List
- `CancelTask(ctx, uid)`: 校验非终态 → TransitionTo(Canceled) → store.Update
- `RerunTask(ctx, uid)`: 复制原任务字段 → NewTask → store.Create
- **测试**: 创建, 获取, 不存在, 取消, 重跑, 分页
- **提交**: `feat: 添加任务业务逻辑层`

---

### 9. API 路由与 Task Handler ✅

**文件**: `internal/api/router.go`, `internal/api/task_handler.go`

- `RegisterRoutes(r, taskSvc, resourceProvider)`: 注册全部 8 个端点
- `createTask`: bind JSON → svc.CreateTask → 201
- `getTask`: :task_uid → svc.GetTask → 200 / 404
- `listTasks`: ?status=&page=&size= → svc.ListTasks → 200
- `cancelTask`: :task_uid → svc.CancelTask → 200 / 404 / 400
- `rerunTask`: :task_uid → svc.RerunTask → 201 / 404
- `getTaskLog`: :task_uid → svc.GetTask → 200 {stdout, stderr}
- **测试**: 创建成功, 参数校验失败(400), 获取, 不存在(404), 列表, 取消, 重跑
- **提交**: `feat: 添加任务 API handler 和路由`

---

### 10. Resource 与 Stats Handler ✅

**文件**: `internal/api/resource_handler.go`

- `resourceProvider` 接口：GetTotalResources() (cpu, memMB int)
- `getResources(rp)`: 返回 `{cpu_cores, memory_mb}`，rp 为 nil 时返回 0
- `getStats(svc)`: 调用 ListTasks 获取全量数据，统计 submitted/success/failed/running/success_rate
- 更新 `router.go` 中 RegisterRoutes 签名，接受 resourceProvider 参数
- **提交**: `feat: 添加资源和统计 API handler`

---

### 11. main.go 组装入口 ✅

**文件**: `cmd/server/main.go`

- 加载配置 → 连接 PostgreSQL → 初始化各层 → 启动 Scheduler goroutine → 创建 Gin router → 启动 HTTP Server
- 优雅关闭：监听 SIGINT/SIGTERM → schedCancel() → srv.Shutdown(30s timeout)
- **提交**: `feat: 添加主入口，组装全部组件并支持优雅关闭`

---

### 12. Dockerfile 与集成测试 ✅

**文件**: `Dockerfile`, `internal/integration/integration_test.go`

- Dockerfile: multi-stage build（golang:1.22-alpine → alpine:3.19）
- 集成测试（build tag: `integration`）：创建并等待完成、超时检测、取消任务、列表分页、重跑
- **提交**: `feat: 添加 Dockerfile 和集成测试`

---

### 13. 全流程验证 ✅

- `go test ./internal/...` — 7 个包全部 PASS
- `go build ./cmd/server` — 编译成功
- `go vet ./...` — 无警告
- 打标签 `v0.1.0`

---

## 验证命令

```bash
# 单元测试
go test ./internal/... -v -count=1

# 编译
go build ./cmd/server

# 静态检查
go vet ./...

# 集成测试（需服务已启动）
go test ./internal/integration/ -v -count=1 -tags=integration

# 一键启动开发环境
./scripts/dev.sh
```
