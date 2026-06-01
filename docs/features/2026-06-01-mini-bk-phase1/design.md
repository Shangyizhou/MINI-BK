# Mini-BK Phase 1 Design

> 详细设计文档见 [`docs/superpowers/specs/2026-06-01-mini-bk-resourceops-design.md`](../../superpowers/specs/2026-06-01-mini-bk-resourceops-design.md)

## 设计概览

- **架构**: 渐进式单体 Go 应用，Gin → Service → Scheduler + Executor + Store(PostgreSQL)
- **并发**: buffered channel 信号量 + goroutine 异步执行
- **调度**: ticker 轮询 + 资源感知
- **持久化**: PostgreSQL 作为历史事实库

## 模块

| 模块 | 路径 | 职责 |
|------|------|------|
| 配置管理 | `internal/config/` | Viper 加载 YAML + MINIBK_ 环境变量覆盖 |
| 任务模型 | `internal/model/` | Task 结构体 + 6 状态状态机 |
| 持久化 | `internal/store/` | PostgreSQL CRUD + 分页 + 按状态查询 |
| 执行器 | `internal/executor/` | os/exec 进程管理，超时/取消 |
| 调度器 | `internal/scheduler/` | ticker 轮询，资源感知调度 |
| 业务逻辑 | `internal/service/` | CreateTask/CancelTask/RerunTask |
| HTTP API | `internal/api/` | Gin router + 8 个端点 |

## API

```
POST   /api/v1/tasks               创建任务
GET    /api/v1/tasks               任务列表
GET    /api/v1/tasks/:task_uid     任务详情
POST   /api/v1/tasks/:task_uid/cancel  取消
POST   /api/v1/tasks/:task_uid/rerun   重跑
GET    /api/v1/tasks/:task_uid/log     日志
GET    /api/v1/resources           资源余量
GET    /api/v1/stats               统计
```
