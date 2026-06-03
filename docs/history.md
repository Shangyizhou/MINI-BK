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

---

## 2026-06-03 — Phase 2 Redis 队列版任务平台

### Redis 集成

- Redis 连接封装：`store.NewRedis()`，支持地址/密码/DB 配置
- Redis 连接失败时优雅降级，不影响核心功能
- 配置项扩展：Redis（addr/password/db）、Retry（max_attempts/initial_interval_sec/max_interval_sec/multiplier）、RateLimit（enabled/requests_per_minute）

### 任务队列 (queue)

- `queue.TaskQueue` 接口抽象：Push / Pop / PushPriority / PushDelayed / Ack / Size / Close
- `queue.InMemQueue`：基于 `container/heap` 的优先级队列 + `sync.Cond` 阻塞 Pop
- InMemQueue 支持延迟队列（最小堆按时间排序）
- `queue.RedisQueue`：基于 Redis List / SortedSet / Stream
- RedisQueue 支持延迟任务（ZSet 按分数轮询）
- 自动选择：Redis 可用用 RedisQueue，否则用 InMemQueue

### 实时日志流 (logstream)

- `logstream.LogStream` 基于 Redis Stream 实现
- `Append()` 写入日志行（MaxLen 10000）
- `Read()` 从指定 ID 开始读取，支持阻塞等待
- Executor 中实时流式输出 stdout/stderr 到 Redis Stream
- SSE 端点 `GET /api/v1/tasks/:task_uid/log/stream` 推送实时日志

### 接口限流 (middleware)

- `middleware.RateLimit` 基于 Redis INCR + TTL，IP 级别限流
- 配置项 `rate_limit.enabled` 开关，`rate_limit.requests_per_minute` 阈值
- 自动注入 X-RateLimit-Limit / X-RateLimit-Remaining / X-RateLimit-Reset / Retry-After 响应头
- Redis 错误时自动放行，不阻塞业务
- 禁用时零开销（直接 c.Next()）

### 幂等性

- Task 模型增加 `IdempotencyKey` 字段（SHA256 命令+工作目录+环境变量 → 16 位 hex）
- 通过 Redis SETNX 5 分钟窗口检查重复
- 重复提交返回错误消息 "duplicate task: <existing_uid>"

### 任务重试

- Task 模型增加 `MaxRetries` / `RetryCount` / `RetryIntervalSec` 字段
- `CanRetry()` 判断是否可重试（RetryCount < MaxRetries）
- 失败任务自动重试：状态回退 Pending，清除 FinishedAt
- 指数退避延迟：`delay = RetryIntervalSec * 2^RetryCount`（上限 5 分钟）
- 超过最大重试次数后标记为 Failed

### 每日统计

- Redis Hash `stats:daily:<YYYY-MM-DD>` 记录 submitted/success/failed
- CreateTask 时 HIncrBy submitted
- 任务成功/失败时分别更新 success/failed
- `GET /api/v1/stats/daily` 查询指定日期的统计
- `GET /api/v1/stats` 合并展示当日统计

### 主入口增强 (main.go)

- 连接 Redis（优雅降级）
- 创建任务队列（Redis → InMem 自动选择）
- 注入限流中间件
- 完整的错误处理和优雅关闭

### 部署

- `deployments/docker-compose.yml`：PostgreSQL 16 + Redis 7 + Server
- `scripts/setup-pg.sh` 扩展：同时启动 Redis 容器
- 环境变量支持：`MINIBK_REDIS_ADDR` 等

### 集成测试

- `TestIdempotency`：幂等性验证
- `TestTaskRetry`：任务重试验证
- `TestSSELogStream`：SSE 日志流验证
- `TestRateLimit`：限流验证
- `TestDelayedTask`：延迟任务验证

### 发布

- Tag: v0.2.0

---

## 2026-06-04 — Phase 3 节点管理与远程执行

### 节点模型与节点存储

- 节点模型 `model.Node`：节点 ID、主机名、IP、版本、标签、资源总量/使用量、心跳时间、状态
- 节点状态机：offline / online / drain
- `store.NodeStore`：CRUD + UpdateHeartbeat + 按状态列表
- 迁移脚本 `migrations/000003_create_nodes`：nodes 表 DDL

### 节点管理器 (nodemanager)

- `NodeManager`：内存缓存在线节点，支持注册、心跳更新
- 注册：gRPC Register → DB Create → 加入在线缓存
- 心跳：每 10 秒更新资源使用+最后心跳时间
- 离线检测：StartOfflineChecker 每 5 秒检查，超时 30 秒标记 Offline
- 标签匹配：FindByLabels 支持 key=value 和松散匹配
- 在线节点列表：GetOnlineNodes() 供调度使用

### gRPC 服务端 (grpcserver)

- `AgentServer` 实现 proto.AgentServiceServer
- Register：接收 Agent 注册请求
- Heartbeat：接收 Agent 心跳 + 资源上报
- PullTask：Agent 轮询拉取待分配任务
- ReportResult：接收 Agent 远程执行结果
- RegisterWithGRPC：注册 gRPC 服务

### gRPC 客户端连接池 (grpcclient)

- `Pool`：按 nodeID 管理 gRPC 连接
- GetOrCreate：延迟创建，双重检查锁
- Remove：关闭并移除连接
- Close：关闭所有连接
- 可选组件，当前 server 侧暂未主动调用

### 节点选择调度

- Scheduler 新增 `SelectNode()` 三层筛选：标签匹配 → 资源过滤 → 最少负载
- `dispatchRemote()`：推送到节点队列供 PullTask 消费
- `GetNextTaskForNode()`：gRPC PullTask 回调
- `HandleRemoteResult()`：处理远程执行结果
- 无匹配节点时自动降级到本地执行
- 节点队列满时降级到本地执行

### 节点管理 API

- `GET /api/v1/nodes`：列出节点（支持 ?status=online 过滤）
- `GET /api/v1/nodes/:node_id`：节点详情
- `POST /api/v1/nodes/:node_id/drain`：设置 Drain 状态
- `POST /api/v1/nodes/:node_id/uncordon`：恢复在线

### 任务模型扩展

- Task 新增 `node_selector` JSONB 字段（标签选择器）
- Task 新增 `assigned_node_id` 字段（分配的节点 ID）
- 迁移脚本 `migrations/000004_add_node_selector`
- NewTask 初始化空 NodeSelector map
- CreateTaskRequest 支持 node_selector 字段

### Agent 二进制 (cmd/agent)

- 独立 Agent 入口，支持 `-server-addr` 和 `-labels` 参数
- 启动时 Register 注册到 Server
- 定时 Heartbeat 上报资源使用
- 定时 PullTask 轮询拉取任务
- 本地 executor.Run() 执行后 ReportResult

### 主入口增强 (main.go)

- gRPC Server 在 :50051 端口监听
- AgentClient 连接池（可选）
- 完整的节点管理 + 离线检测 goroutine
- 优雅关闭顺序：HTTP → gRPC → Scheduler → DB 连接
- 配置扩展：grpc.port / agent.heartbeat_interval_sec / agent.heartbeat_timeout_sec

### 部署

- `Dockerfile.agent`：Agent 独立 multi-stage 构建
- `deployments/docker-compose.yml`：新增 Agent 服务
- `docker-compose.yml` 新增 :50051 端口映射

### 发布

- Tag: v0.3.0
