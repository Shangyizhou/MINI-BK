# Mini-BK ResourceOps 二期实现计划

> **给执行者的说明：** 必须使用 superpowers:subagent-driven-development 按任务逐个实现。步骤使用 checkbox（`- [ ]`）跟踪进度。

**目标：** 任务提交与消费解耦，支持服务重启不丢任务，API Server 可横向扩展。

**架构：** 在 Phase 1 单体基础上引入 Redis（go-redis/v9）作为队列中间层、日志流、限流和临时状态存储。Scheduler 从 Redis 队列 Pop 任务而非直接读 PostgreSQL。

**技术栈：** Go 1.22+, Gin, PostgreSQL, Redis(go-redis/v9), log/slog

**设计文档：** `docs/superpowers/specs/2026-06-01-mini-bk-resourceops-design.md`（§3 二期）

---

### 任务 1: Redis 连接与配置

**涉及文件：**
- 修改: `configs/config.yaml`（新增 redis 和 retry 配置段）
- 新建: `internal/store/redis.go`
- 新建: `internal/store/redis_test.go`

- [ ] **步骤 1: 更新配置**

`configs/config.yaml` 新增：
```yaml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0

retry:
  max_attempts: 3
  initial_interval_sec: 1
  max_interval_sec: 60
  multiplier: 2

rate_limit:
  enabled: true
  requests_per_minute: 100
```

- [ ] **步骤 2: 更新 config 包**

在 `internal/config/config.go` 新增：
```go
type RedisConfig struct {
    Addr     string `mapstructure:"addr"`
    Password string `mapstructure:"password"`
    DB       int    `mapstructure:"db"`
}

type RetryConfig struct {
    MaxAttempts        int `mapstructure:"max_attempts"`
    InitialIntervalSec int `mapstructure:"initial_interval_sec"`
    MaxIntervalSec     int `mapstructure:"max_interval_sec"`
    Multiplier         int `mapstructure:"multiplier"`
}

type RateLimitConfig struct {
    Enabled           bool `mapstructure:"enabled"`
    RequestsPerMinute int  `mapstructure:"requests_per_minute"`
}
```

并在 `Config` 结构体添加 `Redis RedisConfig`、`Retry RetryConfig`、`RateLimit RateLimitConfig` 字段，在 `Load()` 中设置默认值。

- [ ] **步骤 3: 编写 Redis 连接测试（TDD）**

```go
func TestNewRedis(t *testing.T) {
    rdb, err := NewRedis(context.Background(), "localhost:6379", "", 0)
    if err != nil {
        t.Skipf("跳过：无法连接 Redis: %v", err)
    }
    defer rdb.Close()
    // ping
}
```

- [ ] **步骤 4: 实现 Redis 连接**

`internal/store/redis.go`:
```go
func NewRedis(ctx context.Context, addr, password string, db int) (*Redis, error) {
    rdb := redis.NewClient(&redis.Options{...})
    if err := rdb.Ping(ctx).Err(); err != nil { return nil, err }
    return &Redis{Client: rdb}, nil
}
```

- [ ] **步骤 5: 提交** `feat: 添加 Redis 连接管理和配置`

---

### 任务 2: Task 模型扩展（重试 + 幂等）

**涉及文件：**
- 修改: `internal/model/task.go`
- 修改: `migrations/000002_add_retry_fields.up.sql`
- 修改: `internal/model/task_test.go`

- [ ] **步骤 1: 迁移脚本**

```sql
ALTER TABLE tasks ADD COLUMN max_retries INT DEFAULT 3;
ALTER TABLE tasks ADD COLUMN retry_count INT DEFAULT 0;
ALTER TABLE tasks ADD COLUMN retry_interval_sec INT DEFAULT 1;
ALTER TABLE tasks ADD COLUMN idempotency_key VARCHAR(64);
CREATE INDEX idx_tasks_idempotency ON tasks(idempotency_key);
```

- [ ] **步骤 2: 更新 Task 结构体**

新增字段：`MaxRetries int`, `RetryCount int`, `RetryIntervalSec int`, `IdempotencyKey string`

新增方法：
```go
func (t *Task) IdempotencyHash() string {
    // sha256(command + workdir + sorted env keys/values) 前 16 位
}

func (t *Task) CanRetry() bool {
    return t.RetryCount < t.MaxRetries
}

func (t *Task) NextRetryDelay() time.Duration {
    // retry_interval_sec * (multiplier ^ retry_count)，不超过 max_interval_sec
}
```

- [ ] **步骤 3: 更新 NewTask()** 设置默认值 MaxRetries=3, RetryIntervalSec=1

- [ ] **步骤 4: 提交** `feat: 任务模型新增重试和幂等字段`

---

### 任务 3: 队列抽象层 + InMem 实现

**涉及文件：**
- 新建: `internal/queue/queue.go`
- 新建: `internal/queue/inmem.go`
- 新建: `internal/queue/inmem_test.go`

- [ ] **步骤 1: 定义 TaskQueue 接口**

```go
type TaskQueue interface {
    Push(ctx, task) error
    Pop(ctx) (*Task, error)           // 阻塞
    PushPriority(ctx, task) error     // 优先级队列
    PushDelayed(ctx, task, delay) error
    Ack(ctx, taskUID) error
    Size(ctx) (int64, error)
}
```

- [ ] **步骤 2: 实现 InMemQueue**

用 channel + heap 实现优先级队列，用 time.Timer 实现延迟队列。

- [ ] **步骤 3: TDD** 测试 Push/Pop/Ack/Priority/Delayed

- [ ] **步骤 4: 提交** `feat: 添加队列抽象层和 InMem 实现`

---

### 任务 4: Redis 队列实现

**涉及文件：**
- 新建: `internal/queue/redis.go`
- 新建: `internal/queue/redis_test.go`

- [ ] **步骤 1: 实现 RedisQueue**

- `Push`: LPUSH `tasks:queue:pending` + 写入任务 JSON
- `Pop`: BRPOP `tasks:queue:pending`（阻塞 5s，可配置）
- `PushPriority`: ZADD `tasks:queue:priority` score=priority
- `PushDelayed`: ZADD `tasks:queue:delayed` score=now+delay
- `Ack`: 从 processing set 中移除
- 额外的后台 goroutine: 每秒检查 `tasks:queue:delayed` 中到期任务，ZREMRANGEBYSCORE + LPUSH 到 pending

- [ ] **步骤 2: TDD** 需要真实 Redis，无 Redis 时 SKIP

- [ ] **步骤 3: 提交** `feat: 添加 Redis 队列实现`

---

### 任务 5: 日志流（Redis Stream + SSE）

**涉及文件：**
- 新建: `internal/logstream/logstream.go`
- 修改: `internal/api/task_handler.go`（新增 SSE endpoint）

- [ ] **步骤 1: 实现 LogStream**

```go
type LogStream struct {
    rdb *redis.Client
}

func (s *LogStream) Append(ctx, taskUID, line string) error  // XADD
func (s *LogStream) Read(ctx, taskUID, lastID string) ([]LogEntry, error) // XREAD
```

- [ ] **步骤 2: 修改 Executor**，执行过程中将 stdout/stderr 逐行写入 LogStream

- [ ] **步骤 3: 实现 SSE Handler**

`GET /api/v1/tasks/:task_uid/log/stream` — 使用 Gin `c.Stream()` + 定时 XREAD 推送

- [ ] **步骤 4: 提交** `feat: 添加 Redis Stream 日志和 SSE 实时推送`

---

### 任务 6: 接口限流中间件

**涉及文件：**
- 新建: `internal/middleware/ratelimit.go`
- 新建: `internal/middleware/ratelimit_test.go`

- [ ] **步骤 1: 实现限流中间件**

```go
func RateLimit(rdb *redis.Client, requestsPerMinute int) gin.HandlerFunc {
    // key := "ratelimit:<user_id>:<minute_window>"
    // INCR + EXPIRE
    // 超过限制返回 429
}
```

- [ ] **步骤 2: TDD** 用 mock redis 测试

- [ ] **步骤 3: 提交** `feat: 添加 Redis 接口限流中间件`

---

### 任务 7: 任务取消（Redis Pub/Sub）

**涉及文件：**
- 修改: `internal/scheduler/scheduler.go`
- 修改: `internal/service/task_service.go`

- [ ] **步骤 1: CancelTask 发布取消信号**

```go
func (s *TaskService) CancelTask(ctx, uid) error {
    // ... 原有逻辑 ...
    s.rdb.Publish(ctx, "tasks:cancel", uid)  // 通知 Executor
}
```

- [ ] **步骤 2: Scheduler/Executor 订阅取消信号**

在 dispatch 的 goroutine 中订阅 `tasks:cancel:<task_uid>`，收到信号后 cancel context。

- [ ] **步骤 3: 提交** `feat: 添加 Redis Pub/Sub 任务取消通知`

---

### 任务 8: 任务幂等 + 重试逻辑

**涉及文件：**
- 修改: `internal/service/task_service.go`
- 修改: `internal/scheduler/scheduler.go`

- [ ] **步骤 1: CreateTask 幂等检查**

```go
// 计算 idempotency hash
// SETNX tasks:dedup:<hash> <task_uid> EX 300
// 如果 key 已存在，返回 "duplicate task"
```

- [ ] **步骤 2: Scheduler 失败重试**

在 `failTask` 中检查 `task.CanRetry()`：
- 如果可重试：`task.RetryCount++`，`PushDelayed(task, task.NextRetryDelay())`，状态回 Pending
- 如果不可重试：标记 Failed（原有逻辑）

- [ ] **步骤 3: 提交** `feat: 添加任务幂等检查和失败重试逻辑`

---

### 任务 9: 新增 API 端点

**涉及文件：**
- 修改: `internal/api/router.go`
- 修改: `internal/api/task_handler.go`
- 修改: `internal/api/resource_handler.go`

- [ ] **步骤 1: 每日统计端点**

`GET /api/v1/tasks/stats/daily` — 从 Redis `stats:daily:<date>` Hash 读取

- [ ] **步骤 2: 任务统计增强**

在 Executor 完成/失败时 HINCRBY stats:daily 的 submitted/success/failed 字段

- [ ] **步骤 3: 提交** `feat: 添加每日统计和增强的任务指标`

---

### 任务 10: 重构 Scheduler 使用队列

**涉及文件：**
- 修改: `internal/scheduler/scheduler.go`

- [ ] **步骤 1: Scheduler 改为从队列 Pop**

核心变更：
- 去掉 `GetCreatedTasks()` 和 `GetPendingTasks()` 调用
- 改为 `queue.Pop(ctx)` 阻塞获取任务
- Created 任务在 Service 层 Push 到队列
- Scheduler 不再需要处理 Created→Pending 转换

- [ ] **步骤 2: 保留资源感知调度**

Pop 出任务后仍然进行 canAllocate 检查，不满足则 PushDelayed（1s 后重试）

- [ ] **步骤 3: 提交** `feat: 重构调度器使用 Redis 队列替代 PostgreSQL 轮询`

---

### 任务 11: main.go 更新

**涉及文件：**
- 修改: `cmd/server/main.go`

- [ ] **步骤 1: 初始化 Redis**

```go
redisStore, err := store.NewRedis(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
```

- [ ] **步骤 2: 选择队列实现**

```go
var taskQueue queue.TaskQueue
if redisStore != nil {
    taskQueue = queue.NewRedisQueue(redisStore.Client)
} else {
    taskQueue = queue.NewInMemQueue()
}
```

- [ ] **步骤 3: 注入新组件**

Scheduler 接收 TaskQueue + LogStream；Router 添加限流中间件

- [ ] **步骤 4: 提交** `feat: 主入口集成 Redis 和队列`

---

### 任务 12: Docker Compose + 集成测试

**涉及文件：**
- 新建: `deployments/docker-compose.yml`
- 修改: `internal/integration/integration_test.go`

- [ ] **步骤 1: docker-compose.yml**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    ...
  redis:
    image: redis:7-alpine
    ...
  server:
    build: .
    depends_on: [postgres, redis]
    ports: ["8080:8080"]
```

- [ ] **步骤 2: 新增集成测试**

幂等、重试、SSE 日志流、限流、延迟任务

- [ ] **步骤 3: 提交** `feat: 添加 Docker Compose 和 Phase 2 集成测试`

---

### 任务 13: 全流程验证

- 全部测试通过
- 编译成功
- `docker-compose up` 一键启动
- 手动 API 验证
- 打标签 `v0.2.0`
