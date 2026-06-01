# Mini-BK Phase 1 Implementation Plan

> 详细实现计划见 [`docs/superpowers/plans/2026-06-01-mini-bk-phase1-implementation.md`](../../superpowers/plans/2026-06-01-mini-bk-phase1-implementation.md)

## 任务列表

| # | 任务 | 涉及文件 | 状态 |
|---|------|----------|------|
| 1 | 项目骨架初始化 | go.mod, Makefile, 目录结构 | ✅ |
| 2 | 配置管理 | internal/config/ | ✅ |
| 3 | Task 模型与状态机 | internal/model/ | ✅ |
| 4 | 数据库迁移与连接 | migrations/, internal/store/postgres.go | ✅ |
| 5 | TaskStore 持久化层 | internal/store/task_store.go | ✅ |
| 6 | Executor 进程执行器 | internal/executor/ | ✅ |
| 7 | Scheduler 调度器 | internal/scheduler/ | ✅ |
| 8 | TaskService 业务层 | internal/service/ | ✅ |
| 9 | API 路由与 Handler | internal/api/ | ✅ |
| 10 | Resource/Stats Handler | internal/api/resource_handler.go | ✅ |
| 11 | main.go 组装入口 | cmd/server/main.go | ✅ |
| 12 | Dockerfile + 集成测试 | Dockerfile, internal/integration/ | ✅ |
| 13 | 全流程验证 | — | ✅ |

## 验证命令

```bash
go test ./internal/... -v -count=1
go build ./cmd/server
go vet ./...
```
