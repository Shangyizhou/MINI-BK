# History

## 2026-06-01 — Phase 1 单机版资源任务平台

### 项目初始化

- 初始化 Go module `github.com/shangyizhou/mini-bk`
- 搭建目录结构（cmd/server, internal/*, configs, migrations, scripts, deployments）
- 安装核心依赖（Gin, lib/pq, Viper, golang-migrate, uuid）
- 创建 Makefile 和 .gitignore

### 配置管理 (config)

- 实现 Viper 配置加载，支持 YAML 文件 + MINIBK_ 环境变量覆盖
- 配置项：server, database, scheduler, executor 四大块
- 支持 DSN() 和 Addr() 辅助方法

### 任务模型与状态机 (model)

- Task 结构体完整映射 tasks 表
- 6 状态状态机：Created → Pending → Running → Success/Failed/Canceled
- TransitionTo() 校验流转合法性
- IsTerminal() 判断终态
- ErrTaskNotFound sentinel error

### 数据库层 (store)

- PostgreSQL 连接池管理（MaxOpenConns=25, MaxIdleConns=5）
- 迁移脚本：tasks 表 DDL（含 3 个索引）
- TaskStore CRUD：Create, Update, GetByUID, List（分页+按状态筛选）
- 按状态查询：GetCreatedTasks, GetPendingTasks, GetRunningTasks
- JSONB env 字段 marshal/unmarshal

### 进程执行器 (executor)

- buffered channel 信号量控制并发
- os/exec + context.WithTimeout 实现超时 kill
- context cancel 支持任务取消
- 环境变量注入 + 工作目录设置
- stdout/stderr 捕获

### 调度器 (scheduler)

- ticker 轮询（默认 500ms 间隔）
- 资源感知调度：计算已分配 CPU/内存，与可用资源比较
- 调度优先级：Pending > Created，按 priority DESC + created_at ASC
- 资源不足时 Created → Pending
- 超时 Running 任务检测：比较 started_at + timeout_sec 与当前时间
- GetTotalResources() 供 API 查询

### 业务逻辑层 (service)

- CreateTask：填充默认值 + 调用 store.Create
- GetTask / ListTasks：代理 store 查询
- CancelTask：校验非终态 → TransitionTo(Canceled) → store.Update
- RerunTask：复制原任务字段 → 创建新 task_uid

### HTTP API 层 (api)

- 路由：POST/GET /api/v1/tasks, GET /:task_uid, POST /:task_uid/cancel, POST /:task_uid/rerun, GET /:task_uid/log
- GET /api/v1/resources：返回本机 CPU/内存总量
- GET /api/v1/stats：返回提交数/成功数/失败数/成功率
- 错误处理：404（任务不存在）、400（参数校验失败）、500（内部错误）

### 组装入口 (main.go)

- 配置加载 → PostgreSQL 连接 → 各层初始化 → Scheduler 启动 → Gin 路由 → HTTP Server
- 优雅关闭：SIGINT/SIGTERM → 停 Scheduler → HTTP Shutdown（30s 超时）

### 部署

- Dockerfile：multi-stage build（golang:1.22-alpine builder → alpine:3.19 runtime）
- 集成测试：5 个端到端测试（build tag: integration）

### 发布

- Tag: v0.1.0
