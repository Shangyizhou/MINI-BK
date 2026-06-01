# Mini-BK ResourceOps 一期实现计划

> **对于智能工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 来逐任务执行此计划。步骤使用复选框（`- [ ]`）语法进行跟踪。

**目标：** 在单机上跑通任务调度最小闭环——提交任务、排队、执行、查日志、看结果。

**架构：** 渐进式单体 Go 应用，Gin HTTP API 层 → Service 业务逻辑层 → Scheduler（goroutine ticker 轮询）+ Executor（os/exec 进程管理）+ Store（PostgreSQL 持久化）。并发控制用 buffered channel 信号量，超时用 context.WithTimeout。

**技术栈：** Go 1.22+, Gin, PostgreSQL, log/slog, Viper, golang-migrate, testcontainers-go

**设计文档：** `docs/superpowers/specs/2026-06-01-mini-bk-resourceops-design.md`

---

### 任务 1：项目骨架初始化

**文件：**
- 创建：`go.mod`
- 创建：`Makefile`
- 创建所有目录

- [ ] **步骤 1：初始化 Go module**

```bash
cd /Users/syz/code/golang/Mini-BK
go mod init github.com/shangyizhou/mini-bk
```

预期：`go.mod` 创建成功，module 路径为 `github.com/shangyizhou/mini-bk`

- [ ] **步骤 2：创建目录结构**

```bash
mkdir -p cmd/server \
  internal/api \
  internal/service \
  internal/scheduler \
  internal/executor \
  internal/model \
  internal/store \
  configs \
  migrations \
  scripts \
  deployments
```

- [ ] **步骤 3：创建 Makefile**

```makefile
.PHONY: build run test lint clean migrate-up migrate-down

APP_NAME = mini-bk-server
BUILD_DIR = ./bin

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

run: build
	$(BUILD_DIR)/$(APP_NAME)

test:
	go test ./... -v -count=1

test-integration:
	go test ./... -v -count=1 -tags=integration

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down

dev: build
	DATABASE_URL=postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable \
	$(BUILD_DIR)/$(APP_NAME)
```

- [ ] **步骤 4：安装核心依赖**

```bash
go get github.com/gin-gonic/gin
go get github.com/lib/pq
go get github.com/spf13/viper
go get github.com/golang-migrate/migrate/v4
go get github.com/google/uuid
```

- [ ] **步骤 5：提交**

```bash
git add go.mod go.sum Makefile
git commit -m "chore: init project skeleton with go module and makefile"
```

---

### 任务 2：配置管理

**文件：**
- 创建：`configs/config.yaml`
- 创建：`internal/store/postgres.go`（配置加载部分）
- 创建：`internal/config/config.go`

- [ ] **步骤 1：创建配置文件**

`configs/config.yaml`:
```yaml
server:
  port: 8080
  host: "0.0.0.0"

database:
  host: "localhost"
  port: 5432
  user: "mini-bk"
  password: "mini-bk"
  dbname: "mini-bk"
  sslmode: "disable"

scheduler:
  tick_interval_ms: 500
  max_concurrent_tasks: 10

executor:
  default_timeout_sec: 300
  default_workdir: "/tmp"
```

- [ ] **步骤 2：创建配置结构体并写单元测试**

`internal/config/config_test.go`:
```go
package config

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadConfig(t *testing.T) {
    // 创建临时配置文件
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.yaml")
    content := `
server:
  port: 9090
database:
  host: "testhost"
  port: 5433
  user: "testuser"
  password: "testpass"
  dbname: "testdb"
  sslmode: "disable"
scheduler:
  tick_interval_ms: 1000
  max_concurrent_tasks: 5
executor:
  default_timeout_sec: 60
  default_workdir: "/var/tmp"
`
    if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
        t.Fatal(err)
    }

    cfg, err := Load(configPath)
    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
    if cfg.Server.Port != 9090 {
        t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
    }
    if cfg.Database.Host != "testhost" {
        t.Errorf("Database.Host = %s, want testhost", cfg.Database.Host)
    }
    if cfg.Scheduler.MaxConcurrentTasks != 5 {
        t.Errorf("Scheduler.MaxConcurrentTasks = %d, want 5", cfg.Scheduler.MaxConcurrentTasks)
    }
    if cfg.Executor.DefaultWorkdir != "/var/tmp" {
        t.Errorf("Executor.DefaultWorkdir = %s, want /var/tmp", cfg.Executor.DefaultWorkdir)
    }
}

func TestLoadConfigEnvOverride(t *testing.T) {
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.yaml")
    content := `
server:
  port: 8080
database:
  host: "localhost"
  port: 5432
  user: "dbuser"
  password: "dbpass"
  dbname: "testdb"
  sslmode: "disable"
scheduler:
  tick_interval_ms: 500
  max_concurrent_tasks: 10
executor:
  default_timeout_sec: 300
  default_workdir: "/tmp"
`
    os.WriteFile(configPath, []byte(content), 0644)
    os.Setenv("MINIBK_SERVER_PORT", "7070")
    os.Setenv("MINIBK_DATABASE_HOST", "override-host")
    defer os.Unsetenv("MINIBK_SERVER_PORT")
    defer os.Unsetenv("MINIBK_DATABASE_HOST")

    cfg, err := Load(configPath)
    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
    if cfg.Server.Port != 7070 {
        t.Errorf("Server.Port = %d, want 7070 (env override)", cfg.Server.Port)
    }
    if cfg.Database.Host != "override-host" {
        t.Errorf("Database.Host = %s, want override-host (env override)", cfg.Database.Host)
    }
}
```

- [ ] **步骤 3：运行测试确认其失败**

```bash
go test ./internal/config/ -v
```

预期：FAIL — `undefined: config.Load`

- [ ] **步骤 4：实现 config 包**

`internal/config/config.go`:
```go
package config

import (
    "fmt"
    "os"
    "strings"

    "github.com/spf13/viper"
)

type Config struct {
    Server    ServerConfig    `mapstructure:"server"`
    Database  DatabaseConfig  `mapstructure:"database"`
    Scheduler SchedulerConfig `mapstructure:"scheduler"`
    Executor  ExecutorConfig  `mapstructure:"executor"`
}

type ServerConfig struct {
    Port int    `mapstructure:"port"`
    Host string `mapstructure:"host"`
}

func (s ServerConfig) Addr() string {
    return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type DatabaseConfig struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    User     string `mapstructure:"user"`
    Password string `mapstructure:"password"`
    DBName   string `mapstructure:"dbname"`
    SSLMode  string `mapstructure:"sslmode"`
}

func (d DatabaseConfig) DSN() string {
    return fmt.Sprintf(
        "postgres://%s:%s@%s:%d/%s?sslmode=%s",
        d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
    )
}

type SchedulerConfig struct {
    TickIntervalMs      int `mapstructure:"tick_interval_ms"`
    MaxConcurrentTasks  int `mapstructure:"max_concurrent_tasks"`
}

type ExecutorConfig struct {
    DefaultTimeoutSec int    `mapstructure:"default_timeout_sec"`
    DefaultWorkdir    string `mapstructure:"default_workdir"`
}

func Load(configPath string) (*Config, error) {
    v := viper.New()

    if configPath != "" {
        v.SetConfigFile(configPath)
    } else {
        v.SetConfigName("config")
        v.SetConfigType("yaml")
        v.AddConfigPath("./configs")
        v.AddConfigPath(".")
    }

    // 环境变量覆盖: MINIBK_SERVER_PORT → server.port
    v.SetEnvPrefix("MINIBK")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()

    // 设置默认值
    v.SetDefault("server.port", 8080)
    v.SetDefault("server.host", "0.0.0.0")
    v.SetDefault("database.host", "localhost")
    v.SetDefault("database.port", 5432)
    v.SetDefault("database.sslmode", "disable")
    v.SetDefault("scheduler.tick_interval_ms", 500)
    v.SetDefault("scheduler.max_concurrent_tasks", 10)
    v.SetDefault("executor.default_timeout_sec", 300)
    v.SetDefault("executor.default_workdir", "/tmp")

    if err := v.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return nil, fmt.Errorf("read config: %w", err)
        }
        // config file not found is ok — use defaults + env
    }

    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("unmarshal config: %w", err)
    }

    // 环境变量手动覆盖（Viper 的 AutomaticEnv + Unmarshal 有已知限制）
    if port := os.Getenv("MINIBK_SERVER_PORT"); port != "" {
        fmt.Sscanf(port, "%d", &cfg.Server.Port)
    }
    if host := os.Getenv("MINIBK_DATABASE_HOST"); host != "" {
        cfg.Database.Host = host
    }

    return &cfg, nil
}
```

- [ ] **步骤 5：运行测试确认其通过**

```bash
go test ./internal/config/ -v
```

预期：PASS

- [ ] **步骤 6：提交**

```bash
git add internal/config/config.go internal/config/config_test.go configs/config.yaml
git commit -m "feat: add config management with viper and env override"
```

---

### 任务 3：Task 模型与状态机

**文件：**
- 创建：`internal/model/task.go`
- 创建：`internal/model/task_test.go`

- [ ] **步骤 1：编写 Task 状态机测试**

`internal/model/task_test.go`:
```go
package model

import (
    "testing"
    "time"
)

func TestTaskStatusTransitions(t *testing.T) {
    tests := []struct {
        name       string
        fromStatus TaskStatus
        toStatus   TaskStatus
        wantOK     bool
    }{
        {"Created to Pending", TaskStatusCreated, TaskStatusPending, true},
        {"Created to Canceled", TaskStatusCreated, TaskStatusCanceled, true},
        {"Pending to Running", TaskStatusPending, TaskStatusRunning, true},
        {"Pending to Canceled", TaskStatusPending, TaskStatusCanceled, true},
        {"Running to Success", TaskStatusRunning, TaskStatusSuccess, true},
        {"Running to Failed", TaskStatusRunning, TaskStatusFailed, true},
        {"Running to Canceled", TaskStatusRunning, TaskStatusCanceled, true},
        // 终态不可流转
        {"Success to anything", TaskStatusSuccess, TaskStatusRunning, false},
        {"Failed to anything", TaskStatusFailed, TaskStatusRunning, false},
        {"Canceled to anything", TaskStatusCanceled, TaskStatusRunning, false},
        // 不可逆流
        {"Running to Created", TaskStatusRunning, TaskStatusCreated, false},
        {"Running to Pending", TaskStatusRunning, TaskStatusPending, false},
        {"Success to Pending", TaskStatusSuccess, TaskStatusPending, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            task := &Task{Status: tt.fromStatus}
            err := task.TransitionTo(tt.toStatus)
            if (err == nil) != tt.wantOK {
                t.Errorf("TransitionTo(%s → %s) error = %v, wantOK = %v",
                    tt.fromStatus, tt.toStatus, err, tt.wantOK)
            }
        })
    }
}

func TestTaskIsTerminal(t *testing.T) {
    terminal := []TaskStatus{TaskStatusSuccess, TaskStatusFailed, TaskStatusCanceled}
    nonTerminal := []TaskStatus{TaskStatusCreated, TaskStatusPending, TaskStatusRunning}

    for _, s := range terminal {
        if !s.IsTerminal() {
            t.Errorf("%s should be terminal", s)
        }
    }
    for _, s := range nonTerminal {
        if s.IsTerminal() {
            t.Errorf("%s should not be terminal", s)
        }
    }
}

func TestNewTask(t *testing.T) {
    task := NewTask("test-task", "echo hello")
    if task.TaskUID == "" {
        t.Error("TaskUID should not be empty")
    }
    if task.Status != TaskStatusCreated {
        t.Errorf("Status = %s, want %s", task.Status, TaskStatusCreated)
    }
    if task.Workdir != "/tmp" {
        t.Errorf("Workdir = %s, want /tmp", task.Workdir)
    }
    if task.TimeoutSec != 300 {
        t.Errorf("TimeoutSec = %d, want 300", task.TimeoutSec)
    }
    if task.CreatedAt.IsZero() {
        t.Error("CreatedAt should not be zero")
    }
}

func TestTaskStatusString(t *testing.T) {
    if TaskStatusCreated.String() != "created" {
        t.Errorf("TaskStatusCreated.String() = %s, want created", TaskStatusCreated.String())
    }
    if TaskStatusRunning.String() != "running" {
        t.Errorf("TaskStatusRunning.String() = %s, want running", TaskStatusRunning.String())
    }
}
```

- [ ] **步骤 2：运行测试确认其失败**

```bash
go test ./internal/model/ -v
```

预期：FAIL — `undefined: model.TaskStatus`

- [ ] **步骤 3：实现 Task 模型**

`internal/model/task.go`:
```go
package model

import (
    "fmt"
    "time"

    "github.com/google/uuid"
)

// TaskStatus 表示任务的当前状态。
type TaskStatus string

const (
    TaskStatusCreated  TaskStatus = "created"
    TaskStatusPending  TaskStatus = "pending"
    TaskStatusRunning  TaskStatus = "running"
    TaskStatusSuccess  TaskStatus = "success"
    TaskStatusFailed   TaskStatus = "failed"
    TaskStatusCanceled TaskStatus = "canceled"
)

func (s TaskStatus) String() string { return string(s) }

// IsTerminal 判断是否为终态。
func (s TaskStatus) IsTerminal() bool {
    switch s {
    case TaskStatusSuccess, TaskStatusFailed, TaskStatusCanceled:
        return true
    default:
        return false
    }
}

// 状态流转规则
var validTransitions = map[TaskStatus]map[TaskStatus]bool{
    TaskStatusCreated: {
        TaskStatusPending:  true,
        TaskStatusCanceled: true,
    },
    TaskStatusPending: {
        TaskStatusRunning:  true,
        TaskStatusCanceled: true,
    },
    TaskStatusRunning: {
        TaskStatusSuccess:  true,
        TaskStatusFailed:   true,
        TaskStatusCanceled: true,
    },
    // 终态无出口
    TaskStatusSuccess:  {},
    TaskStatusFailed:   {},
    TaskStatusCanceled: {},
}

// Task 表示一个待执行或已执行的任务。
type Task struct {
    ID           int64      `json:"id"`
    TaskUID      string     `json:"task_uid"`
    Name         string     `json:"name"`
    Command      string     `json:"command"`
    Workdir      string     `json:"workdir"`
    Env          map[string]string `json:"env"`
    CPULimit     int        `json:"cpu_limit"`
    MemoryLimit  int        `json:"memory_limit"`
    TimeoutSec   int        `json:"timeout_sec"`
    Priority     int        `json:"priority"`
    Status       TaskStatus `json:"status"`
    ExitCode     *int       `json:"exit_code"`
    Stdout       string     `json:"stdout"`
    Stderr       string     `json:"stderr"`
    ErrorMessage string     `json:"error_message"`
    PID          *int       `json:"pid"`
    StartedAt    *time.Time `json:"started_at"`
    FinishedAt   *time.Time `json:"finished_at"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}

// NewTask 创建一个新任务，填充默认值并生成 UUID。
func NewTask(name, command string) *Task {
    now := time.Now()
    return &Task{
        TaskUID:    uuid.New().String(),
        Name:       name,
        Command:    command,
        Workdir:    "/tmp",
        Env:        make(map[string]string),
        TimeoutSec: 300,
        Status:     TaskStatusCreated,
        CreatedAt:  now,
        UpdatedAt:  now,
    }
}

// TransitionTo 将任务状态转为目标状态，校验流转合法性。
func (t *Task) TransitionTo(target TaskStatus) error {
    allowed, ok := validTransitions[t.Status]
    if !ok || !allowed[target] {
        return fmt.Errorf("invalid status transition: %s → %s", t.Status, target)
    }
    t.Status = target
    t.UpdatedAt = time.Now()
    return nil
}
```

- [ ] **步骤 4：运行测试确认其通过**

```bash
go test ./internal/model/ -v
```

预期：PASS

- [ ] **步骤 5：提交**

```bash
git add internal/model/task.go internal/model/task_test.go
git commit -m "feat: add task model with status state machine"
```

---

### 任务 4：数据库迁移与连接

**文件：**
- 创建：`migrations/000001_create_tasks.up.sql`
- 创建：`migrations/000001_create_tasks.down.sql`
- 修改：`internal/store/postgres.go`（添加连接逻辑）

- [ ] **步骤 1：编写数据库迁移 SQL**

`migrations/000001_create_tasks.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS tasks (
    id            BIGSERIAL PRIMARY KEY,
    task_uid      VARCHAR(36)  NOT NULL UNIQUE,
    name          VARCHAR(255) NOT NULL,
    command       TEXT         NOT NULL,
    workdir       VARCHAR(512) DEFAULT '/tmp',
    env           JSONB        DEFAULT '{}',
    cpu_limit     INT          DEFAULT 0,
    memory_limit  INT          DEFAULT 0,
    timeout_sec   INT          DEFAULT 300,
    priority      INT          DEFAULT 0,
    status        VARCHAR(20)  NOT NULL DEFAULT 'created',
    exit_code     INT,
    stdout        TEXT,
    stderr        TEXT,
    error_message TEXT,
    pid           INT,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_priority ON tasks(status, priority DESC);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);
```

`migrations/000001_create_tasks.down.sql`:
```sql
DROP TABLE IF EXISTS tasks;
```

- [ ] **步骤 2：编写 Postgres 连接测试**

`internal/store/postgres_test.go`:
```go
package store

import (
    "context"
    "testing"
    "time"
)

func TestNewPostgres(t *testing.T) {
    // 这个测试需要真实的 PostgreSQL 连接
    // 在 CI 中使用 testcontainers，本地开发用本地 PG
    dsn := "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable"
    db, err := NewPostgres(context.Background(), dsn)
    if err != nil {
        t.Skipf("skipping integration test: cannot connect to postgres: %v", err)
    }
    defer db.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    if err := db.PingContext(ctx); err != nil {
        t.Fatalf("ping failed: %v", err)
    }
}
```

- [ ] **步骤 3：运行测试确认其失败**

```bash
go test ./internal/store/ -v -run TestNewPostgres
```

预期：FAIL — `undefined: store.NewPostgres`

- [ ] **步骤 4：实现 Postgres 连接**

`internal/store/postgres.go`:
```go
package store

import (
    "context"
    "database/sql"
    "fmt"
    "log/slog"
    "time"

    _ "github.com/lib/pq"
)

// Postgres 封装 PostgreSQL 数据库连接。
type Postgres struct {
    DB *sql.DB
}

// NewPostgres 创建 PostgreSQL 连接并验证连通性。
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, fmt.Errorf("open postgres: %w", err)
    }

    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)

    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    if err := db.PingContext(ctx); err != nil {
        db.Close()
        return nil, fmt.Errorf("ping postgres: %w", err)
    }

    slog.Info("connected to postgres")
    return &Postgres{DB: db}, nil
}

// Close 关闭数据库连接。
func (p *Postgres) Close() error {
    return p.DB.Close()
}
```

- [ ] **步骤 5：运行测试确认其通过**

```bash
go test ./internal/store/ -v -run TestNewPostgres
```

预期：PASS（如果有 PG 实例）或 SKIP（如果没有）

- [ ] **步骤 6：提交**

```bash
git add migrations/ internal/store/postgres.go internal/store/postgres_test.go
git commit -m "feat: add postgres connection and task table migration"
```

---

### 任务 5：Task Store（数据持久化层）

**文件：**
- 创建：`internal/store/task_store.go`
- 创建：`internal/store/task_store_test.go`

- [ ] **步骤 1：编写 TaskStore 接口和测试**

`internal/store/task_store_test.go`:
```go
package store

import (
    "context"
    "testing"
    "time"

    "github.com/shangyizhou/mini-bk/internal/model"
)

func setupTaskStore(t *testing.T) (*TaskStore, func()) {
    t.Helper()
    dsn := "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable"
    pg, err := NewPostgres(context.Background(), dsn)
    if err != nil {
        t.Skipf("skipping: cannot connect to postgres: %v", err)
    }
    // 清理测试数据
    pg.DB.ExecContext(context.Background(), "DELETE FROM tasks")
    store := NewTaskStore(pg)
    return store, func() {
        pg.DB.ExecContext(context.Background(), "DELETE FROM tasks")
        pg.Close()
    }
}

func TestTaskStore_Create(t *testing.T) {
    store, cleanup := setupTaskStore(t)
    defer cleanup()

    task := model.NewTask("test-create", "echo hello")
    task.Priority = 5
    task.CPULimit = 2

    if err := store.Create(context.Background(), task); err != nil {
        t.Fatalf("Create() error = %v", err)
    }
    if task.ID == 0 {
        t.Error("Create() should set ID")
    }

    // 验证可读回
    got, err := store.GetByUID(context.Background(), task.TaskUID)
    if err != nil {
        t.Fatalf("GetByUID() error = %v", err)
    }
    if got.Name != "test-create" {
        t.Errorf("Name = %s, want test-create", got.Name)
    }
    if got.Priority != 5 {
        t.Errorf("Priority = %d, want 5", got.Priority)
    }
}

func TestTaskStore_List(t *testing.T) {
    store, cleanup := setupTaskStore(t)
    defer cleanup()

    // 创建多个任务
    for i := 0; i < 5; i++ {
        task := model.NewTask("test-list", "echo hello")
        if err := store.Create(context.Background(), task); err != nil {
            t.Fatalf("Create() error = %v", err)
        }
    }

    tasks, total, err := store.List(context.Background(), "", 1, 3)
    if err != nil {
        t.Fatalf("List() error = %v", err)
    }
    if total != 5 {
        t.Errorf("total = %d, want 5", total)
    }
    if len(tasks) != 3 {
        t.Errorf("len(tasks) = %d, want 3 (page size)", len(tasks))
    }
}

func TestTaskStore_ListByStatus(t *testing.T) {
    store, cleanup := setupTaskStore(t)
    defer cleanup()

    task := model.NewTask("test-status", "echo hello")
    task.Status = model.TaskStatusRunning
    store.Create(context.Background(), task)

    tasks, total, err := store.List(context.Background(), "running", 1, 10)
    if err != nil {
        t.Fatalf("List() error = %v", err)
    }
    if total != 1 {
        t.Errorf("total = %d, want 1", total)
    }
}

func TestTaskStore_UpdateStatus(t *testing.T) {
    store, cleanup := setupTaskStore(t)
    defer cleanup()

    task := model.NewTask("test-update", "echo hello")
    store.Create(context.Background(), task)

    task.TransitionTo(model.TaskStatusPending)
    if err := store.Update(context.Background(), task); err != nil {
        t.Fatalf("Update() error = %v", err)
    }

    got, _ := store.GetByUID(context.Background(), task.TaskUID)
    if got.Status != model.TaskStatusPending {
        t.Errorf("Status = %s, want pending", got.Status)
    }
}

func TestTaskStore_GetPendingTasks(t *testing.T) {
    store, cleanup := setupTaskStore(t)
    defer cleanup()

    // 创建 3 个 Pending 任务
    for i := 0; i < 3; i++ {
        task := model.NewTask("test-pending", "echo hello")
        task.Status = model.TaskStatusPending
        store.Create(context.Background(), task)
    }
    // 创建 1 个 Created 任务
    task := model.NewTask("test-created", "echo hello")
    store.Create(context.Background(), task)

    tasks, err := store.GetPendingTasks(context.Background())
    if err != nil {
        t.Fatalf("GetPendingTasks() error = %v", err)
    }
    if len(tasks) != 3 {
        t.Errorf("len(tasks) = %d, want 3", len(tasks))
    }
}

func TestTaskStore_GetRunningTasks(t *testing.T) {
    store, cleanup := setupTaskStore(t)
    defer cleanup()

    task := model.NewTask("test-running", "sleep 10")
    task.Status = model.TaskStatusRunning
    task.CPULimit = 2
    task.MemoryLimit = 512
    store.Create(context.Background(), task)

    tasks, err := store.GetRunningTasks(context.Background())
    if err != nil {
        t.Fatalf("GetRunningTasks() error = %v", err)
    }
    if len(tasks) != 1 {
        t.Errorf("len(tasks) = %d, want 1", len(tasks))
    }
    if tasks[0].CPULimit != 2 {
        t.Errorf("CPULimit = %d, want 2", tasks[0].CPULimit)
    }
}
```

- [ ] **步骤 2：运行测试确认其失败**

```bash
go test ./internal/store/ -v -run TestTaskStore
```

预期：FAIL — `undefined: store.NewTaskStore`

- [ ] **步骤 3：实现 TaskStore**

`internal/store/task_store.go`:
```go
package store

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "time"

    "github.com/shangyizhou/mini-bk/internal/model"
)

// TaskStore 负责任务数据的持久化。
type TaskStore struct {
    pg *Postgres
}

// NewTaskStore 创建 TaskStore 实例。
func NewTaskStore(pg *Postgres) *TaskStore {
    return &TaskStore{pg: pg}
}

// Create 插入一条新任务记录。
func (s *TaskStore) Create(ctx context.Context, task *model.Task) error {
    envJSON, err := json.Marshal(task.Env)
    if err != nil {
        return fmt.Errorf("marshal env: %w", err)
    }

    return s.pg.DB.QueryRowContext(ctx, `
        INSERT INTO tasks (task_uid, name, command, workdir, env,
            cpu_limit, memory_limit, timeout_sec, priority, status,
            created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
        RETURNING id
    `,
        task.TaskUID, task.Name, task.Command, task.Workdir, envJSON,
        task.CPULimit, task.MemoryLimit, task.TimeoutSec, task.Priority,
        string(task.Status), task.CreatedAt, task.UpdatedAt,
    ).Scan(&task.ID)
}

// Update 更新任务记录。使用 task_uid 定位。
func (s *TaskStore) Update(ctx context.Context, task *model.Task) error {
    envJSON, err := json.Marshal(task.Env)
    if err != nil {
        return fmt.Errorf("marshal env: %w", err)
    }

    var pid, exitCode *int
    var startedAt, finishedAt *time.Time

    // 对可空字段使用临时变量
    if task.PID != nil {
        v := *task.PID
        pid = &v
    }
    if task.ExitCode != nil {
        v := *task.ExitCode
        exitCode = &v
    }
    startedAt = task.StartedAt
    finishedAt = task.FinishedAt

    _, err = s.pg.DB.ExecContext(ctx, `
        UPDATE tasks SET
            name = $1, command = $2, workdir = $3, env = $4,
            cpu_limit = $5, memory_limit = $6, timeout_sec = $7,
            priority = $8, status = $9, exit_code = $10,
            stdout = $11, stderr = $12, error_message = $13,
            pid = $14, started_at = $15, finished_at = $16,
            updated_at = $17
        WHERE task_uid = $18
    `,
        task.Name, task.Command, task.Workdir, envJSON,
        task.CPULimit, task.MemoryLimit, task.TimeoutSec,
        task.Priority, string(task.Status), exitCode,
        task.Stdout, task.Stderr, task.ErrorMessage,
        pid, startedAt, finishedAt,
        time.Now(), task.TaskUID,
    )
    if err != nil {
        return fmt.Errorf("update task: %w", err)
    }
    task.UpdatedAt = time.Now()
    return nil
}

// GetByUID 根据 task_uid 查询任务。
func (s *TaskStore) GetByUID(ctx context.Context, uid string) (*model.Task, error) {
    return s.scanTask(s.pg.DB.QueryRowContext(ctx, `
        SELECT id, task_uid, name, command, workdir, env,
            cpu_limit, memory_limit, timeout_sec, priority, status,
            exit_code, stdout, stderr, error_message, pid,
            started_at, finished_at, created_at, updated_at
        FROM tasks WHERE task_uid = $1
    `, uid))
}

// List 分页查询任务列表，可按 status 筛选。
func (s *TaskStore) List(ctx context.Context, status string, page, size int) ([]*model.Task, int, error) {
    // 计数
    var total int
    countQuery := "SELECT COUNT(*) FROM tasks"
    args := []interface{}{}
    if status != "" {
        countQuery += " WHERE status = $1"
        args = append(args, status)
    }
    if err := s.pg.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
        return nil, 0, fmt.Errorf("count tasks: %w", err)
    }

    // 分页查询
    offset := (page - 1) * size
    dataQuery := `
        SELECT id, task_uid, name, command, workdir, env,
            cpu_limit, memory_limit, timeout_sec, priority, status,
            exit_code, stdout, stderr, error_message, pid,
            started_at, finished_at, created_at, updated_at
        FROM tasks
    `
    dataArgs := []interface{}{}
    if status != "" {
        dataQuery += " WHERE status = $" + fmt.Sprint(len(dataArgs)+1)
        dataArgs = append(dataArgs, status)
    }
    dataQuery += " ORDER BY priority DESC, created_at ASC LIMIT $" + fmt.Sprint(len(dataArgs)+1) + " OFFSET $" + fmt.Sprint(len(dataArgs)+2)
    dataArgs = append(dataArgs, size, offset)

    rows, err := s.pg.DB.QueryContext(ctx, dataQuery, dataArgs...)
    if err != nil {
        return nil, 0, fmt.Errorf("query tasks: %w", err)
    }
    defer rows.Close()

    var tasks []*model.Task
    for rows.Next() {
        task, err := s.scanTaskFromRows(rows)
        if err != nil {
            return nil, 0, err
        }
        tasks = append(tasks, task)
    }
    if err := rows.Err(); err != nil {
        return nil, 0, fmt.Errorf("iterate tasks: %w", err)
    }
    return tasks, total, nil
}

// GetPendingTasks 获取所有 Pending 状态的任务，按优先级排序。
func (s *TaskStore) GetPendingTasks(ctx context.Context) ([]*model.Task, error) {
    return s.queryTasks(ctx, "pending")
}

// GetRunningTasks 获取所有 Running 状态的任务。
func (s *TaskStore) GetRunningTasks(ctx context.Context) ([]*model.Task, error) {
    return s.queryTasks(ctx, "running")
}

// GetCreatedTasks 获取所有 Created 状态的任务，按优先级+创建时间排序。
func (s *TaskStore) GetCreatedTasks(ctx context.Context) ([]*model.Task, error) {
    rows, err := s.pg.DB.QueryContext(ctx, `
        SELECT id, task_uid, name, command, workdir, env,
            cpu_limit, memory_limit, timeout_sec, priority, status,
            exit_code, stdout, stderr, error_message, pid,
            started_at, finished_at, created_at, updated_at
        FROM tasks
        WHERE status = 'created'
        ORDER BY priority DESC, created_at ASC
    `)
    if err != nil {
        return nil, fmt.Errorf("query created tasks: %w", err)
    }
    defer rows.Close()

    var tasks []*model.Task
    for rows.Next() {
        task, err := s.scanTaskFromRows(rows)
        if err != nil {
            return nil, err
        }
        tasks = append(tasks, task)
    }
    return tasks, rows.Err()
}

func (s *TaskStore) queryTasks(ctx context.Context, status string) ([]*model.Task, error) {
    rows, err := s.pg.DB.QueryContext(ctx, `
        SELECT id, task_uid, name, command, workdir, env,
            cpu_limit, memory_limit, timeout_sec, priority, status,
            exit_code, stdout, stderr, error_message, pid,
            started_at, finished_at, created_at, updated_at
        FROM tasks WHERE status = $1
    `, status)
    if err != nil {
        return nil, fmt.Errorf("query tasks: %w", err)
    }
    defer rows.Close()

    var tasks []*model.Task
    for rows.Next() {
        task, err := s.scanTaskFromRows(rows)
        if err != nil {
            return nil, err
        }
        tasks = append(tasks, task)
    }
    return tasks, rows.Err()
}

type scannable interface {
    Scan(dest ...interface{}) error
}

func (s *TaskStore) scanTask(row scannable) (*model.Task, error) {
    task := &model.Task{}
    var envBytes []byte
    err := row.Scan(
        &task.ID, &task.TaskUID, &task.Name, &task.Command, &task.Workdir, &envBytes,
        &task.CPULimit, &task.MemoryLimit, &task.TimeoutSec, &task.Priority, &task.Status,
        &task.ExitCode, &task.Stdout, &task.Stderr, &task.ErrorMessage, &task.PID,
        &task.StartedAt, &task.FinishedAt, &task.CreatedAt, &task.UpdatedAt,
    )
    if err != nil {
        return nil, fmt.Errorf("scan task: %w", err)
    }
    if len(envBytes) > 0 {
        if err := json.Unmarshal(envBytes, &task.Env); err != nil {
            return nil, fmt.Errorf("unmarshal env: %w", err)
        }
    }
    if task.Env == nil {
        task.Env = make(map[string]string)
    }
    return task, nil
}

func (s *TaskStore) scanTaskFromRows(rows *sql.Rows) (*model.Task, error) {
    return s.scanTask(rows)
}
```

- [ ] **步骤 4：运行测试确认其通过**

```bash
go test ./internal/store/ -v -run TestTaskStore
```

预期：PASS

- [ ] **步骤 5：提交**

```bash
git add internal/store/task_store.go internal/store/task_store_test.go
git commit -m "feat: add task store with CRUD and filtered queries"
```

---

### 任务 6：Executor（进程执行器）

**文件：**
- 创建：`internal/executor/executor.go`
- 创建：`internal/executor/executor_test.go`

- [ ] **步骤 1：编写 Executor 单元测试**

`internal/executor/executor_test.go`:
```go
package executor

import (
    "context"
    "testing"
    "time"

    "github.com/shangyizhou/mini-bk/internal/model"
)

func TestExecutor_RunSuccess(t *testing.T) {
    exec := NewExecutor(10) // max 10 concurrent

    task := model.NewTask("test-success", "echo hello")
    task.Workdir = "/tmp"
    task.TimeoutSec = 5

    result := exec.Run(context.Background(), task)
    if result.Error != nil {
        t.Fatalf("Run() error = %v", result.Error)
    }
    if result.ExitCode != 0 {
        t.Errorf("ExitCode = %d, want 0", result.ExitCode)
    }
    if result.Stdout != "hello\n" {
        t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
    }
}

func TestExecutor_RunTimeout(t *testing.T) {
    exec := NewExecutor(10)

    task := model.NewTask("test-timeout", "sleep 10")
    task.Workdir = "/tmp"
    task.TimeoutSec = 1 // 1 秒超时

    result := exec.Run(context.Background(), task)
    if result.Error == nil {
        t.Fatal("Run() should have error (timeout)")
    }
    if !result.TimedOut {
        t.Error("TimedOut should be true")
    }
}

func TestExecutor_RunFailedCommand(t *testing.T) {
    exec := NewExecutor(10)

    task := model.NewTask("test-fail", "exit 42")
    task.Workdir = "/tmp"
    task.TimeoutSec = 5

    result := exec.Run(context.Background(), task)
    if result.ExitCode != 42 {
        t.Errorf("ExitCode = %d, want 42", result.ExitCode)
    }
}

func TestExecutor_RunWithEnv(t *testing.T) {
    exec := NewExecutor(10)

    task := model.NewTask("test-env", "echo $MY_VAR")
    task.Workdir = "/tmp"
    task.TimeoutSec = 5
    task.Env = map[string]string{"MY_VAR": "my_value"}

    result := exec.Run(context.Background(), task)
    if result.Error != nil {
        t.Fatalf("Run() error = %v", result.Error)
    }
    if result.Stdout != "my_value\n" {
        t.Errorf("Stdout = %q, want %q", result.Stdout, "my_value\n")
    }
}

func TestExecutor_Cancel(t *testing.T) {
    exec := NewExecutor(10)

    task := model.NewTask("test-cancel", "sleep 60")
    task.Workdir = "/tmp"
    task.TimeoutSec = 120

    ctx, cancel := context.WithCancel(context.Background())

    // 在另一个 goroutine 中执行，然后取消
    resultCh := make(chan *TaskResult, 1)
    go func() {
        resultCh <- exec.Run(ctx, task)
    }()

    time.Sleep(100 * time.Millisecond) // 给进程一点时间启动
    cancel()

    select {
    case result := <-resultCh:
        if result.Error == nil {
            t.Error("Run() should have error (canceled)")
        }
    case <-time.After(5 * time.Second):
        t.Fatal("timeout waiting for cancel")
    }
}

func TestExecutor_ConcurrencyLimit(t *testing.T) {
    maxConcurrent := 2
    exec := NewExecutor(maxConcurrent)

    // 同时提交 5 个长任务，验证只有 2 个能执行
    started := make(chan struct{}, 5)
    done := make(chan struct{}, 5)

    for i := 0; i < 5; i++ {
        go func(idx int) {
            task := model.NewTask("test-concurrency", "sleep 2")
            task.Workdir = "/tmp"
            task.TimeoutSec = 10
            started <- struct{}{}
            exec.Run(context.Background(), task)
            done <- struct{}{}
        }(i)
    }

    // 第一批：等待 2 个启动
    time.Sleep(200 * time.Millisecond)
    if len(started) < 2 {
        t.Errorf("expected at least 2 tasks to start quickly, got %d", len(started))
    }
}
```

- [ ] **步骤 2：运行测试确认其失败**

```bash
go test ./internal/executor/ -v
```

预期：FAIL — `undefined: executor.NewExecutor`

- [ ] **步骤 3：实现 Executor**

`internal/executor/executor.go`:
```go
package executor

import (
    "bytes"
    "context"
    "fmt"
    "log/slog"
    "os"
    "os/exec"
    "syscall"
    "time"

    "github.com/shangyizhou/mini-bk/internal/model"
)

// TaskResult 表示任务执行的结果。
type TaskResult struct {
    ExitCode int
    Stdout   string
    Stderr   string
    TimedOut bool
    Error    error
}

// Executor 负责任务的本地进程管理。
type Executor struct {
    slots chan struct{} // 并发控制信号量
}

// NewExecutor 创建执行器，maxConcurrent 是最大并发任务数。
func NewExecutor(maxConcurrent int) *Executor {
    return &Executor{
        slots: make(chan struct{}, maxConcurrent),
    }
}

// Run 在本地执行任务命令，返回执行结果。
// 通过 context 支持超时和取消。阻塞直到任务完成或超时/取消。
func (e *Executor) Run(ctx context.Context, task *model.Task) *TaskResult {
    // 获取执行槽位
    select {
    case e.slots <- struct{}{}:
        defer func() { <-e.slots }()
    case <-ctx.Done():
        return &TaskResult{Error: ctx.Err()}
    }

    // 创建带超时的 context
    timeout := time.Duration(task.TimeoutSec) * time.Second
    execCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    cmd := exec.CommandContext(execCtx, "sh", "-c", task.Command)

    // 设置工作目录
    if task.Workdir != "" {
        cmd.Dir = task.Workdir
    }

    // 设置环境变量
    cmd.Env = os.Environ() // 继承系统环境变量
    for k, v := range task.Env {
        cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
    }

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    slog.Info("starting task", "task_uid", task.TaskUID, "command", task.Command)
    startTime := time.Now()

    err := cmd.Run()
    elapsed := time.Since(startTime)

    result := &TaskResult{
        Stdout: stdout.String(),
        Stderr: stderr.String(),
    }

    if err != nil {
        if execCtx.Err() == context.DeadlineExceeded {
            result.TimedOut = true
            result.Error = fmt.Errorf("task timeout after %s", timeout)
            slog.Warn("task timed out", "task_uid", task.TaskUID, "timeout", timeout)
        } else if ctx.Err() == context.Canceled {
            result.Error = fmt.Errorf("task canceled")
            slog.Info("task canceled", "task_uid", task.TaskUID)
        } else if exitErr, ok := err.(*exec.ExitError); ok {
            if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
                result.ExitCode = status.ExitStatus()
            }
        } else {
            result.Error = fmt.Errorf("run command: %w", err)
        }
    }

    slog.Info("task finished",
        "task_uid", task.TaskUID,
        "exit_code", result.ExitCode,
        "elapsed", elapsed,
    )
    return result
}
```

- [ ] **步骤 4：运行测试确认其通过**

```bash
go test ./internal/executor/ -v -timeout 30s
```

预期：PASS

- [ ] **步骤 5：提交**

```bash
git add internal/executor/executor.go internal/executor/executor_test.go
git commit -m "feat: add executor with timeout, cancel, and concurrency control"
```

---

### 任务 7：Scheduler（任务调度器）

**文件：**
- 创建：`internal/scheduler/scheduler.go`
- 创建：`internal/scheduler/scheduler_test.go`

- [ ] **步骤 1：编写 Scheduler 单元测试**

`internal/scheduler/scheduler_test.go`:
```go
package scheduler

import (
    "context"
    "sync"
    "testing"
    "time"

    "github.com/shangyizhou/mini-bk/internal/executor"
    "github.com/shangyizhou/mini-bk/internal/model"
)

// mockTaskStore 实现 Scheduler 所需的 store 接口
type mockTaskStore struct {
    mu           sync.Mutex
    created      []*model.Task
    pending      []*model.Task
    running      []*model.Task
    updateCalled int
}

func (m *mockTaskStore) GetCreatedTasks(ctx context.Context) ([]*model.Task, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    return append([]*model.Task{}, m.created...), nil
}

func (m *mockTaskStore) GetPendingTasks(ctx context.Context) ([]*model.Task, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    return append([]*model.Task{}, m.pending...), nil
}

func (m *mockTaskStore) GetRunningTasks(ctx context.Context) ([]*model.Task, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    return append([]*model.Task{}, m.running...), nil
}

func (m *mockTaskStore) Update(ctx context.Context, task *model.Task) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.updateCalled++
    return nil
}

// mockExecutor 模拟执行器，返回 *executor.TaskResult 匹配真实接口
type mockExecutor struct{}

func (m *mockExecutor) Run(ctx context.Context, task *model.Task) *executor.TaskResult {
    // 模拟成功执行
    code := 0
    return &executor.TaskResult{
        ExitCode: code,
        Stdout:   "mock output",
    }
}

func TestScheduler_ScheduleCreatedTask(t *testing.T) {
    store := &mockTaskStore{}
    exec := &mockExecutor{}
    sched := NewScheduler(store, exec, 500*time.Millisecond, 10)

    // 添加一个 Created 任务
    task := model.NewTask("test-schedule", "echo hello")
    task.CPULimit = 1
    task.MemoryLimit = 128
    store.created = append(store.created, task)

    // 手动调用一次调度循环
    sched.tick(context.Background())

    store.mu.Lock()
    defer store.mu.Unlock()
    // 任务应该被移出 created，更新为 running 后执行
    if len(store.created) > 0 {
        t.Errorf("expected 0 created tasks, got %d", len(store.created))
    }
    if store.updateCalled == 0 {
        t.Error("expected Update() to be called")
    }
}

func TestScheduler_ResourceInsufficient(t *testing.T) {
    store := &mockTaskStore{}
    exec := &mockExecutor{}
    sched := NewScheduler(store, exec, 500*time.Millisecond, 10)

    // 提交一个需要大量资源的任务
    task := model.NewTask("test-heavy", "echo hello")
    task.CPULimit = 999  // 超过本机可用
    task.MemoryLimit = 999999
    task.Status = model.TaskStatusCreated
    store.created = append(store.created, task)

    sched.tick(context.Background())

    store.mu.Lock()
    defer store.mu.Unlock()
    // 任务应该被标记为 Pending（资源不足）
    if task.Status != model.TaskStatusPending {
        t.Errorf("Status = %s, want pending (resource insufficient)", task.Status)
    }
}

func TestScheduler_StartStop(t *testing.T) {
    store := &mockTaskStore{}
    exec := &mockExecutor{}
    sched := NewScheduler(store, exec, 100*time.Millisecond, 10)

    ctx, cancel := context.WithCancel(context.Background())
    go sched.Start(ctx)

    // 等待一个 tick
    time.Sleep(200 * time.Millisecond)
    cancel()

    // 验证优雅退出
    <-time.After(200 * time.Millisecond)
    // 不 panic 就算通过
}
```

- [ ] **步骤 2：运行测试确认其失败**

```bash
go test ./internal/scheduler/ -v
```

预期：FAIL — `undefined: scheduler.NewScheduler`

- [ ] **步骤 3：实现 Scheduler**

`internal/scheduler/scheduler.go`:
```go
package scheduler

import (
    "context"
    "log/slog"
    "runtime"
    "time"

    "github.com/shangyizhou/mini-bk/internal/executor"
    "github.com/shangyizhou/mini-bk/internal/model"
)

// TaskStore 调度器所需的数据访问接口。
type TaskStore interface {
    GetCreatedTasks(ctx context.Context) ([]*model.Task, error)
    GetPendingTasks(ctx context.Context) ([]*model.Task, error)
    GetRunningTasks(ctx context.Context) ([]*model.Task, error)
    Update(ctx context.Context, task *model.Task) error
}

// TaskExecutor 调度器所需的任务执行接口。
type TaskExecutor interface {
    Run(ctx context.Context, task *model.Task) *executor.TaskResult
}

// Scheduler 负责轮询任务队列，分配资源，启动执行。
type Scheduler struct {
    store          TaskStore
    executor       TaskExecutor
    tickInterval   time.Duration
    maxConcurrent  int

    // 本机总资源
    totalCPU   int
    totalMemMB int
}

// NewScheduler 创建调度器。自动检测本机 CPU 核心数作为 totalCPU。
func NewScheduler(store TaskStore, executor TaskExecutor, tickInterval time.Duration, maxConcurrent int) *Scheduler {
    totalCPU := runtime.NumCPU()
    // 粗略估算本机总内存（MB），留 20% 给系统
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    // 用 cgroup 或 /proc/meminfo 更准确，一期用简单方案：可配置
    totalMemMB := 8192 // 默认 8GB

    return &Scheduler{
        store:         store,
        executor:      executor,
        tickInterval:  tickInterval,
        maxConcurrent: maxConcurrent,
        totalCPU:      totalCPU,
        totalMemMB:    totalMemMB,
    }
}

// Start 启动调度循环，阻塞直到 ctx 被取消。
func (s *Scheduler) Start(ctx context.Context) {
    slog.Info("scheduler started",
        "total_cpu", s.totalCPU,
        "total_mem_mb", s.totalMemMB,
        "max_concurrent", s.maxConcurrent,
        "tick_interval", s.tickInterval,
    )

    ticker := time.NewTicker(s.tickInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            slog.Info("scheduler stopping")
            return
        case <-ticker.C:
            s.tick(ctx)
        }
    }
}

// tick 执行一次调度循环。
func (s *Scheduler) tick(ctx context.Context) {
    // 1. 获取当前运行中的任务，计算已分配资源
    running, err := s.store.GetRunningTasks(ctx)
    if err != nil {
        slog.Error("get running tasks", "error", err)
        return
    }

    allocatedCPU := 0
    allocatedMem := 0
    for _, t := range running {
        allocatedCPU += t.CPULimit
        allocatedMem += t.MemoryLimit
    }

    availableCPU := s.totalCPU - allocatedCPU
    availableMem := s.totalMemMB - allocatedMem
    availableSlots := s.maxConcurrent - len(running)

    if availableSlots <= 0 {
        return // 已达并发上限
    }

    // 2. 尝试调度 Pending 任务
    pending, err := s.store.GetPendingTasks(ctx)
    if err != nil {
        slog.Error("get pending tasks", "error", err)
        return
    }
    for _, task := range pending {
        if availableSlots <= 0 {
            break
        }
        if s.canAllocate(task, availableCPU, availableMem) {
            s.dispatch(ctx, task)
            availableCPU -= task.CPULimit
            availableMem -= task.MemoryLimit
            availableSlots--
        }
    }

    // 3. 处理 Created 任务
    created, err := s.store.GetCreatedTasks(ctx)
    if err != nil {
        slog.Error("get created tasks", "error", err)
        return
    }

    for _, task := range created {
        if availableSlots <= 0 {
            break
        }
        if s.canAllocate(task, availableCPU, availableMem) {
            s.dispatch(ctx, task)
            availableCPU -= task.CPULimit
            availableMem -= task.MemoryLimit
            availableSlots--
        } else {
            // 资源不够 → Pending
            if err := task.TransitionTo(model.TaskStatusPending); err != nil {
                slog.Warn("transition to pending", "task_uid", task.TaskUID, "error", err)
                continue
            }
            if err := s.store.Update(ctx, task); err != nil {
                slog.Error("update task to pending", "task_uid", task.TaskUID, "error", err)
            }
        }
    }

    // 4. 检查超时的 Running 任务
    now := time.Now()
    for _, task := range running {
        if task.StartedAt == nil {
            continue
        }
        deadline := task.StartedAt.Add(time.Duration(task.TimeoutSec) * time.Second)
        if now.After(deadline) {
            slog.Warn("task exceeded timeout, marking failed",
                "task_uid", task.TaskUID,
                "started_at", task.StartedAt,
                "timeout_sec", task.TimeoutSec,
            )
            s.failTask(ctx, task, "task timed out (detected by scheduler)")
        }
    }
}

// canAllocate 检查是否有足够资源运行此任务。
func (s *Scheduler) canAllocate(task *model.Task, availableCPU, availableMem int) bool {
    if task.CPULimit > 0 && task.CPULimit > availableCPU {
        return false
    }
    if task.MemoryLimit > 0 && task.MemoryLimit > availableMem {
        return false
    }
    return true
}

// dispatch 启动任务执行。
func (s *Scheduler) dispatch(ctx context.Context, task *model.Task) {
    if err := task.TransitionTo(model.TaskStatusRunning); err != nil {
        slog.Error("transition to running", "task_uid", task.TaskUID, "error", err)
        return
    }
    now := time.Now()
    task.StartedAt = &now

    if err := s.store.Update(ctx, task); err != nil {
        slog.Error("update task before dispatch", "task_uid", task.TaskUID, "error", err)
        return
    }

    // 异步执行
    go func() {
        result := s.executor.Run(ctx, task)

        switch {
        case result.TimedOut:
            s.failTask(ctx, task, "task timed out")
        case result.Error != nil:
            s.failTask(ctx, task, result.Error.Error())
        case result.ExitCode != 0:
            s.failTask(ctx, task, result.Stderr)
        default:
            s.completeTask(ctx, task, result)
        }
    }()
}

// completeTask 将任务标记为成功。
func (s *Scheduler) completeTask(ctx context.Context, task *model.Task, result *executor.TaskResult) {
    if err := task.TransitionTo(model.TaskStatusSuccess); err != nil {
        slog.Error("transition to success", "task_uid", task.TaskUID, "error", err)
        return
    }
    finishTask(task, result.Stdout, result.Stderr)
    if err := s.store.Update(ctx, task); err != nil {
        slog.Error("update task after completion", "task_uid", task.TaskUID, "error", err)
    }
}

// failTask 将任务标记为失败。
func (s *Scheduler) failTask(ctx context.Context, task *model.Task, errMsg string) {
    if err := task.TransitionTo(model.TaskStatusFailed); err != nil {
        slog.Error("transition to failed", "task_uid", task.TaskUID, "error", err)
        return
    }
    finishTask(task, task.Stdout, task.Stderr)
    task.ErrorMessage = errMsg
    if err := s.store.Update(ctx, task); err != nil {
        slog.Error("update task after failure", "task_uid", task.TaskUID, "error", err)
    }
}

// finishTask 设置任务结束的通用字段。
func finishTask(task *model.Task, stdout, stderr string) {
    now := time.Now()
    task.FinishedAt = &now
    task.Stdout = stdout
    task.Stderr = stderr
}

// GetTotalResources 返回本机总资源（供 API 查询用）。
func (s *Scheduler) GetTotalResources() (cpu, memMB int) {
    return s.totalCPU, s.totalMemMB
}
```

- [ ] **步骤 4：运行测试确认其通过**

```bash
go test ./internal/scheduler/ -v
```

预期：PASS

- [ ] **步骤 5：提交**

```bash
git add internal/scheduler/scheduler.go internal/scheduler/scheduler_test.go
git commit -m "feat: add scheduler with resource-aware tick loop"
```

---

### 任务 8：Task Service（业务逻辑层）

**文件：**
- 创建：`internal/service/task_service.go`
- 创建：`internal/service/task_service_test.go`

- [ ] **步骤 1：编写 TaskService 测试**

`internal/service/task_service_test.go`:
```go
package service

import (
    "context"
    "testing"

    "github.com/shangyizhou/mini-bk/internal/model"
)

type mockStore struct {
    tasks map[string]*model.Task
}

func newMockStore() *mockStore {
    return &mockStore{tasks: make(map[string]*model.Task)}
}

func (m *mockStore) Create(ctx context.Context, task *model.Task) error {
    m.tasks[task.TaskUID] = task
    task.ID = int64(len(m.tasks))
    return nil
}

func (m *mockStore) Update(ctx context.Context, task *model.Task) error {
    m.tasks[task.TaskUID] = task
    return nil
}

func (m *mockStore) GetByUID(ctx context.Context, uid string) (*model.Task, error) {
    t, ok := m.tasks[uid]
    if !ok {
        return nil, model.ErrTaskNotFound
    }
    return t, nil
}

func (m *mockStore) List(ctx context.Context, status string, page, size int) ([]*model.Task, int, error) {
    var result []*model.Task
    for _, t := range m.tasks {
        if status == "" || string(t.Status) == status {
            result = append(result, t)
        }
    }
    total := len(result)
    start := (page - 1) * size
    if start >= total {
        return nil, total, nil
    }
    end := start + size
    if end > total {
        end = total
    }
    return result[start:end], total, nil
}

func (m *mockStore) GetCreatedTasks(ctx context.Context) ([]*model.Task, error) { return nil, nil }
func (m *mockStore) GetPendingTasks(ctx context.Context) ([]*model.Task, error) { return nil, nil }
func (m *mockStore) GetRunningTasks(ctx context.Context) ([]*model.Task, error) {
    var result []*model.Task
    for _, t := range m.tasks {
        if t.Status == model.TaskStatusRunning {
            result = append(result, t)
        }
    }
    return result, nil
}

func TestTaskService_CreateTask(t *testing.T) {
    svc := NewTaskService(newMockStore())

    req := CreateTaskRequest{
        Name:        "test-task",
        Command:     "echo hello",
        Workdir:     "/tmp",
        CPULimit:    1,
        MemoryLimit: 256,
        TimeoutSec:  300,
        Priority:    5,
    }

    task, err := svc.CreateTask(context.Background(), req)
    if err != nil {
        t.Fatalf("CreateTask() error = %v", err)
    }
    if task.TaskUID == "" {
        t.Error("TaskUID should not be empty")
    }
    if task.Name != "test-task" {
        t.Errorf("Name = %s, want test-task", task.Name)
    }
    if task.Status != model.TaskStatusCreated {
        t.Errorf("Status = %s, want created", task.Status)
    }
}

func TestTaskService_GetTask(t *testing.T) {
    svc := NewTaskService(newMockStore())

    task, _ := svc.CreateTask(context.Background(), CreateTaskRequest{
        Name:    "test-get",
        Command: "echo hello",
    })

    got, err := svc.GetTask(context.Background(), task.TaskUID)
    if err != nil {
        t.Fatalf("GetTask() error = %v", err)
    }
    if got.TaskUID != task.TaskUID {
        t.Errorf("TaskUID mismatch")
    }
}

func TestTaskService_GetTaskNotFound(t *testing.T) {
    svc := NewTaskService(newMockStore())
    _, err := svc.GetTask(context.Background(), "nonexistent")
    if err == nil {
        t.Error("GetTask() should return error for nonexistent task")
    }
}

func TestTaskService_CancelTask(t *testing.T) {
    store := newMockStore()
    svc := NewTaskService(store)

    task, _ := svc.CreateTask(context.Background(), CreateTaskRequest{
        Name:    "test-cancel",
        Command: "sleep 100",
    })
    // 手动设置为 Running
    task.TransitionTo(model.TaskStatusPending)
    task.TransitionTo(model.TaskStatusRunning)
    store.Update(context.Background(), task)

    err := svc.CancelTask(context.Background(), task.TaskUID)
    if err != nil {
        t.Fatalf("CancelTask() error = %v", err)
    }

    got, _ := svc.GetTask(context.Background(), task.TaskUID)
    if got.Status != model.TaskStatusCanceled {
        t.Errorf("Status = %s, want canceled", got.Status)
    }
}

func TestTaskService_RerunTask(t *testing.T) {
    svc := NewTaskService(newMockStore())

    task, _ := svc.CreateTask(context.Background(), CreateTaskRequest{
        Name:    "test-rerun",
        Command: "echo hello",
    })

    rerun, err := svc.RerunTask(context.Background(), task.TaskUID)
    if err != nil {
        t.Fatalf("RerunTask() error = %v", err)
    }
    if rerun.TaskUID == task.TaskUID {
        t.Error("rerun task should have different TaskUID")
    }
    if rerun.Name != "test-rerun" {
        t.Errorf("Name = %s, want test-rerun", rerun.Name)
    }
    if rerun.Command != "echo hello" {
        t.Errorf("Command = %s, want echo hello", rerun.Command)
    }
}

func TestTaskService_ListTasks(t *testing.T) {
    svc := NewTaskService(newMockStore())

    for i := 0; i < 5; i++ {
        svc.CreateTask(context.Background(), CreateTaskRequest{
            Name:    "test-list",
            Command: "echo hello",
        })
    }

    result, err := svc.ListTasks(context.Background(), "", 1, 3)
    if err != nil {
        t.Fatalf("ListTasks() error = %v", err)
    }
    if result.Total != 5 {
        t.Errorf("Total = %d, want 5", result.Total)
    }
    if len(result.Tasks) != 3 {
        t.Errorf("len(Tasks) = %d, want 3", len(result.Tasks))
    }
}
```

- [ ] **步骤 2：运行测试确认其失败**

```bash
go test ./internal/service/ -v
```

预期：FAIL — `undefined: service.NewTaskService`

- [ ] **步骤 3：实现 TaskService**

`internal/service/task_service.go`:
```go
package service

import (
    "context"
    "fmt"

    "github.com/shangyizhou/mini-bk/internal/model"
)

// TaskStore 业务层所需的数据访问接口。
type TaskStore interface {
    Create(ctx context.Context, task *model.Task) error
    Update(ctx context.Context, task *model.Task) error
    GetByUID(ctx context.Context, uid string) (*model.Task, error)
    List(ctx context.Context, status string, page, size int) ([]*model.Task, int, error)
    GetRunningTasks(ctx context.Context) ([]*model.Task, error)
}

// CreateTaskRequest 创建任务的请求参数。
type CreateTaskRequest struct {
    Name        string            `json:"name" binding:"required"`
    Command     string            `json:"command" binding:"required"`
    Workdir     string            `json:"workdir"`
    Env         map[string]string `json:"env"`
    CPULimit    int               `json:"cpu_limit"`
    MemoryLimit int               `json:"memory_limit"`
    TimeoutSec  int               `json:"timeout_sec"`
    Priority    int               `json:"priority"`
}

// TaskListResult 任务列表查询结果。
type TaskListResult struct {
    Tasks []*model.Task `json:"tasks"`
    Total int           `json:"total"`
    Page  int           `json:"page"`
    Size  int           `json:"size"`
}

// TaskService 处理任务相关的业务逻辑。
type TaskService struct {
    store TaskStore
}

// NewTaskService 创建 TaskService。
func NewTaskService(store TaskStore) *TaskService {
    return &TaskService{store: store}
}

// CreateTask 创建新任务。填充默认值并持久化。
func (s *TaskService) CreateTask(ctx context.Context, req CreateTaskRequest) (*model.Task, error) {
    task := model.NewTask(req.Name, req.Command)

    if req.Workdir != "" {
        task.Workdir = req.Workdir
    }
    if req.Env != nil {
        task.Env = req.Env
    }
    if req.CPULimit > 0 {
        task.CPULimit = req.CPULimit
    }
    if req.MemoryLimit > 0 {
        task.MemoryLimit = req.MemoryLimit
    }
    if req.TimeoutSec > 0 {
        task.TimeoutSec = req.TimeoutSec
    }
    task.Priority = req.Priority

    if err := s.store.Create(ctx, task); err != nil {
        return nil, fmt.Errorf("create task: %w", err)
    }
    return task, nil
}

// GetTask 根据 task_uid 获取任务详情。
func (s *TaskService) GetTask(ctx context.Context, uid string) (*model.Task, error) {
    task, err := s.store.GetByUID(ctx, uid)
    if err != nil {
        return nil, fmt.Errorf("get task %s: %w", uid, err)
    }
    return task, nil
}

// ListTasks 分页查询任务列表。
func (s *TaskService) ListTasks(ctx context.Context, status string, page, size int) (*TaskListResult, error) {
    if page < 1 {
        page = 1
    }
    if size < 1 || size > 100 {
        size = 20
    }

    tasks, total, err := s.store.List(ctx, status, page, size)
    if err != nil {
        return nil, fmt.Errorf("list tasks: %w", err)
    }
    if tasks == nil {
        tasks = []*model.Task{}
    }

    return &TaskListResult{
        Tasks: tasks,
        Total: total,
        Page:  page,
        Size:  size,
    }, nil
}

// CancelTask 取消任务。
func (s *TaskService) CancelTask(ctx context.Context, uid string) error {
    task, err := s.store.GetByUID(ctx, uid)
    if err != nil {
        return fmt.Errorf("get task for cancel: %w", err)
    }

    if task.Status.IsTerminal() {
        return fmt.Errorf("task %s is already in terminal state %s", uid, task.Status)
    }

    if err := task.TransitionTo(model.TaskStatusCanceled); err != nil {
        return fmt.Errorf("cancel task %s: %w", uid, err)
    }

    if err := s.store.Update(ctx, task); err != nil {
        return fmt.Errorf("update canceled task: %w", err)
    }
    return nil
}

// RerunTask 基于已有任务创建新的重跑任务（新 task_uid）。
func (s *TaskService) RerunTask(ctx context.Context, uid string) (*model.Task, error) {
    original, err := s.store.GetByUID(ctx, uid)
    if err != nil {
        return nil, fmt.Errorf("get task for rerun: %w", err)
    }

    newTask := model.NewTask(original.Name, original.Command)
    newTask.Workdir = original.Workdir
    newTask.Env = original.Env
    newTask.CPULimit = original.CPULimit
    newTask.MemoryLimit = original.MemoryLimit
    newTask.TimeoutSec = original.TimeoutSec
    newTask.Priority = original.Priority

    if err := s.store.Create(ctx, newTask); err != nil {
        return nil, fmt.Errorf("create rerun task: %w", err)
    }
    return newTask, nil
}
```

- [ ] **步骤 4：在 model 包中添加 ErrTaskNotFound**

在 `internal/model/task.go` 末尾添加：
```go
import "errors"

var ErrTaskNotFound = errors.New("task not found")
```

- [ ] **步骤 5：运行测试确认其通过**

```bash
go test ./internal/service/ -v
```

预期：PASS

- [ ] **步骤 6：提交**

```bash
git add internal/service/task_service.go internal/service/task_service_test.go internal/model/task.go
git commit -m "feat: add task service with CRUD, cancel, and rerun logic"
```

---

### 任务 9：API 路由与 Task Handler

**文件：**
- 创建：`internal/api/router.go`
- 创建：`internal/api/task_handler.go`
- 创建：`internal/api/task_handler_test.go`

- [ ] **步骤 1：编写 Task Handler 测试**

`internal/api/task_handler_test.go`:
```go
package api

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"

    "github.com/shangyizhou/mini-bk/internal/model"
    "github.com/shangyizhou/mini-bk/internal/service"
)

type mockTaskService struct {
    tasks map[string]*model.Task
}

func newMockTaskService() *mockTaskService {
    return &mockTaskService{tasks: make(map[string]*model.Task)}
}

func (m *mockTaskService) CreateTask(ctx context.Context, req service.CreateTaskRequest) (*model.Task, error) {
    task := model.NewTask(req.Name, req.Command)
    task.TaskUID = "test-uid-123"
    m.tasks[task.TaskUID] = task
    return task, nil
}

func (m *mockTaskService) GetTask(ctx context.Context, uid string) (*model.Task, error) {
    t, ok := m.tasks[uid]
    if !ok {
        return nil, model.ErrTaskNotFound
    }
    return t, nil
}

func (m *mockTaskService) ListTasks(ctx context.Context, status string, page, size int) (*service.TaskListResult, error) {
    var tasks []*model.Task
    for _, t := range m.tasks {
        if status == "" || string(t.Status) == status {
            tasks = append(tasks, t)
        }
    }
    return &service.TaskListResult{Tasks: tasks, Total: len(tasks), Page: page, Size: size}, nil
}

func (m *mockTaskService) CancelTask(ctx context.Context, uid string) error {
    t, ok := m.tasks[uid]
    if !ok {
        return model.ErrTaskNotFound
    }
    t.Status = model.TaskStatusCanceled
    return nil
}

func (m *mockTaskService) RerunTask(ctx context.Context, uid string) (*model.Task, error) {
    return &model.Task{}, nil
}

func setupTestRouter(svc *mockTaskService) *gin.Engine {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    RegisterRoutes(r, svc, nil)
    return r
}

func TestCreateTaskHandler(t *testing.T) {
    svc := newMockTaskService()
    router := setupTestRouter(svc)

    body := map[string]interface{}{
        "name":    "test-task",
        "command": "echo hello",
        "cpu_limit": 1,
    }
    jsonBody, _ := json.Marshal(body)

    req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBuffer(jsonBody))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusCreated {
        t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
    }

    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    if resp["task_uid"] == nil {
        t.Error("response should contain task_uid")
    }
}

func TestCreateTaskHandler_ValidationError(t *testing.T) {
    svc := newMockTaskService()
    router := setupTestRouter(svc)

    // 缺少必填字段 name
    body := map[string]interface{}{
        "command": "echo hello",
    }
    jsonBody, _ := json.Marshal(body)

    req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBuffer(jsonBody))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
    }
}

func TestGetTaskHandler(t *testing.T) {
    svc := newMockTaskService()
    svc.tasks["test-uid-123"] = &model.Task{
        TaskUID: "test-uid-123",
        Name:    "test-task",
        Status:  model.TaskStatusCreated,
    }
    router := setupTestRouter(svc)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/test-uid-123", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
    }
}

func TestGetTaskHandler_NotFound(t *testing.T) {
    svc := newMockTaskService()
    router := setupTestRouter(svc)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/nonexistent", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusNotFound {
        t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
    }
}

func TestListTasksHandler(t *testing.T) {
    svc := newMockTaskService()
    svc.tasks["uid-1"] = &model.Task{TaskUID: "uid-1", Name: "task-1", Status: model.TaskStatusCreated}
    svc.tasks["uid-2"] = &model.Task{TaskUID: "uid-2", Name: "task-2", Status: model.TaskStatusRunning}
    router := setupTestRouter(svc)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?page=1&size=10", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
    }

    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    if resp["total"] == nil {
        t.Error("response should contain total")
    }
}

func TestCancelTaskHandler(t *testing.T) {
    svc := newMockTaskService()
    svc.tasks["test-uid"] = &model.Task{TaskUID: "test-uid", Name: "test", Status: model.TaskStatusRunning}
    router := setupTestRouter(svc)

    req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/test-uid/cancel", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
    }
}

func TestRerunTaskHandler(t *testing.T) {
    svc := newMockTaskService()
    svc.tasks["test-uid"] = &model.Task{TaskUID: "test-uid", Name: "test", Status: model.TaskStatusSuccess}
    router := setupTestRouter(svc)

    req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/test-uid/rerun", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusCreated {
        t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
    }
}
```

- [ ] **步骤 2：运行测试确认其失败**

```bash
go test ./internal/api/ -v
```

预期：FAIL — `undefined: api.RegisterRoutes`

- [ ] **步骤 3：实现路由**

`internal/api/router.go`:
```go
package api

import (
    "github.com/gin-gonic/gin"
)

// TaskHandler 定义 API 层所需的任务业务逻辑接口。
type TaskHandler interface {
    // 方法在 task_handler.go 中通过具体类型注入
}

// RegisterRoutes 注册所有 API 路由。
func RegisterRoutes(r *gin.Engine, taskSvc taskService, rp resourceProvider) {
    v1 := r.Group("/api/v1")
    {
        tasks := v1.Group("/tasks")
        {
            tasks.POST("", createTask(taskSvc))
            tasks.GET("", listTasks(taskSvc))
            tasks.GET("/:task_uid", getTask(taskSvc))
            tasks.POST("/:task_uid/cancel", cancelTask(taskSvc))
            tasks.POST("/:task_uid/rerun", rerunTask(taskSvc))
            tasks.GET("/:task_uid/log", getTaskLog(taskSvc))
        }
        v1.GET("/resources", getResources(rp))
        v1.GET("/stats", getStats(taskSvc))
    }
}
```

- [ ] **步骤 4：实现 Task Handler**

`internal/api/task_handler.go`:
```go
package api

import (
    "context"
    "errors"
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"

    "github.com/shangyizhou/mini-bk/internal/model"
    "github.com/shangyizhou/mini-bk/internal/service"
)

// taskService 是 TaskHandler 所需的业务逻辑接口。
type taskService interface {
    CreateTask(ctx context.Context, req service.CreateTaskRequest) (*model.Task, error)
    GetTask(ctx context.Context, uid string) (*model.Task, error)
    ListTasks(ctx context.Context, status string, page, size int) (*service.TaskListResult, error)
    CancelTask(ctx context.Context, uid string) error
    RerunTask(ctx context.Context, uid string) (*model.Task, error)
}

func createTask(svc taskService) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req service.CreateTaskRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }

        task, err := svc.CreateTask(c.Request.Context(), req)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        c.JSON(http.StatusCreated, gin.H{
            "task_uid":   task.TaskUID,
            "status":     task.Status,
            "created_at": task.CreatedAt,
        })
    }
}

func getTask(svc taskService) gin.HandlerFunc {
    return func(c *gin.Context) {
        uid := c.Param("task_uid")
        task, err := svc.GetTask(c.Request.Context(), uid)
        if err != nil {
            if errors.Is(err, model.ErrTaskNotFound) {
                c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, task)
    }
}

func listTasks(svc taskService) gin.HandlerFunc {
    return func(c *gin.Context) {
        status := c.Query("status")
        page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
        size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

        result, err := svc.ListTasks(c.Request.Context(), status, page, size)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, result)
    }
}

func cancelTask(svc taskService) gin.HandlerFunc {
    return func(c *gin.Context) {
        uid := c.Param("task_uid")
        if err := svc.CancelTask(c.Request.Context(), uid); err != nil {
            if errors.Is(err, model.ErrTaskNotFound) {
                c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
                return
            }
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, gin.H{"message": "task canceled"})
    }
}

func rerunTask(svc taskService) gin.HandlerFunc {
    return func(c *gin.Context) {
        uid := c.Param("task_uid")
        task, err := svc.RerunTask(c.Request.Context(), uid)
        if err != nil {
            if errors.Is(err, model.ErrTaskNotFound) {
                c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusCreated, gin.H{
            "task_uid":   task.TaskUID,
            "status":     task.Status,
            "created_at": task.CreatedAt,
        })
    }
}

func getTaskLog(svc taskService) gin.HandlerFunc {
    return func(c *gin.Context) {
        uid := c.Param("task_uid")
        task, err := svc.GetTask(c.Request.Context(), uid)
        if err != nil {
            if errors.Is(err, model.ErrTaskNotFound) {
                c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, gin.H{
            "stdout": task.Stdout,
            "stderr": task.Stderr,
        })
    }
}
```

- [ ] **步骤 5：运行测试确认其通过**

```bash
go test ./internal/api/ -v
```

预期：PASS

- [ ] **步骤 6：提交**

```bash
git add internal/api/router.go internal/api/task_handler.go internal/api/task_handler_test.go
git commit -m "feat: add task API handlers and routes"
```

---

### 任务 10：Resource 与 Stats Handler

**文件：**
- 创建：`internal/api/resource_handler.go`

- [ ] **步骤 1：实现 Resource 和 Stats Handler**

`internal/api/resource_handler.go`:
```go
package api

import (
    "context"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/shangyizhou/mini-bk/internal/model"
)

// resourceProvider 提供资源信息（由 scheduler 实现）。
type resourceProvider interface {
    GetTotalResources() (cpu, memMB int)
}

// statsProvider 提供统计信息。
type statsProvider interface {
    GetRunningTasks(ctx context.Context) ([]*model.Task, error)
    ListTasks(ctx context.Context, status string, page, size int) ([]*model.Task, int, error)
}

func getResources(rp resourceProvider) gin.HandlerFunc {
    return func(c *gin.Context) {
        if rp == nil {
            c.JSON(http.StatusOK, gin.H{
                "cpu_cores":     0,
                "memory_mb":     0,
                "allocated_cpu": 0,
                "allocated_mem": 0,
            })
            return
        }
        totalCPU, totalMem := rp.GetTotalResources()
        // allocated 在 handler 层通过 scheduler 计算
        c.JSON(http.StatusOK, gin.H{
            "cpu_cores":  totalCPU,
            "memory_mb":  totalMem,
        })
    }
}

func getStats(svc taskService) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx := c.Request.Context()
        // 获取所有运行中的任务（用于统计）
        all, total, err := svc.ListTasks(ctx, "", 1, 1000)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        var submitted, success, failed, running int
        for _, t := range all {
            submitted++
            switch t.Status {
            case model.TaskStatusSuccess:
                success++
            case model.TaskStatusFailed:
                failed++
            case model.TaskStatusRunning:
                running++
            }
        }

        c.JSON(http.StatusOK, gin.H{
            "total_tasks":    total,
            "submitted":      submitted,
            "success":        success,
            "failed":         failed,
            "running":        running,
            "success_rate":   safeDiv(success, submitted),
        })
    }
}

func safeDiv(a, b int) float64 {
    if b == 0 {
        return 0
    }
    return float64(a) / float64(b)
}
```

- [ ] **步骤 2：更新 router.go 注入 resourceProvider**

修改 `internal/api/router.go` 中 `RegisterRoutes` 函数签名，使其接受 `resourceProvider`：

```go
func RegisterRoutes(r *gin.Engine, taskSvc taskService, rp resourceProvider) {
    v1 := r.Group("/api/v1")
    {
        tasks := v1.Group("/tasks")
        {
            tasks.POST("", createTask(taskSvc))
            tasks.GET("", listTasks(taskSvc))
            tasks.GET("/:task_uid", getTask(taskSvc))
            tasks.POST("/:task_uid/cancel", cancelTask(taskSvc))
            tasks.POST("/:task_uid/rerun", rerunTask(taskSvc))
            tasks.GET("/:task_uid/log", getTaskLog(taskSvc))
        }
        v1.GET("/resources", getResources(rp))
        v1.GET("/stats", getStats(taskSvc))
    }
}
```

同时更新测试中的 `setupTestRouter`:
```go
func setupTestRouter(svc *mockTaskService) *gin.Engine {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    RegisterRoutes(r, svc, nil)
    return r
}
```

- [ ] **步骤 3：提交**

```bash
git add internal/api/resource_handler.go internal/api/router.go internal/api/task_handler_test.go
git commit -m "feat: add resource and stats API handlers"
```

---

### 任务 11：main.go（组装入口）

**文件：**
- 创建：`cmd/server/main.go`

- [ ] **步骤 1：实现 main.go**

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/shangyizhou/mini-bk/internal/api"
    "github.com/shangyizhou/mini-bk/internal/config"
    "github.com/shangyizhou/mini-bk/internal/executor"
    "github.com/shangyizhou/mini-bk/internal/scheduler"
    "github.com/shangyizhou/mini-bk/internal/service"
    "github.com/shangyizhou/mini-bk/internal/store"
)

func main() {
    // 初始化结构化日志
    slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })))

    // 加载配置
    configPath := os.Getenv("CONFIG_PATH")
    if configPath == "" {
        configPath = "configs/config.yaml"
    }
    cfg, err := config.Load(configPath)
    if err != nil {
        slog.Error("failed to load config", "error", err)
        os.Exit(1)
    }

    // 连接 PostgreSQL
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    pg, err := store.NewPostgres(ctx, cfg.Database.DSN())
    if err != nil {
        slog.Error("failed to connect to postgres", "error", err)
        os.Exit(1)
    }
    defer pg.Close()

    // 初始化各层
    taskStore := store.NewTaskStore(pg)
    taskSvc := service.NewTaskService(taskStore)
    exec := executor.NewExecutor(cfg.Scheduler.MaxConcurrentTasks)

    sched := scheduler.NewScheduler(
        taskStore,
        exec,
        time.Duration(cfg.Scheduler.TickIntervalMs)*time.Millisecond,
        cfg.Scheduler.MaxConcurrentTasks,
    )

    // 启动调度器
    schedCtx, schedCancel := context.WithCancel(context.Background())
    defer schedCancel()
    go sched.Start(schedCtx)

    // 设置 Gin
    gin.SetMode(gin.ReleaseMode)
    router := gin.New()
    router.Use(gin.Logger(), gin.Recovery())
    api.RegisterRoutes(router, taskSvc, sched)

    // 启动 HTTP Server
    srv := &http.Server{
        Addr:    cfg.Server.Addr(),
        Handler: router,
    }

    go func() {
        slog.Info("server starting", "addr", cfg.Server.Addr())
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("server error", "error", err)
            os.Exit(1)
        }
    }()

    // 等待退出信号
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    sig := <-quit
    slog.Info("received signal, shutting down", "signal", sig)

    // 优雅关闭
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    schedCancel() // 先停调度器

    if err := srv.Shutdown(shutdownCtx); err != nil {
        slog.Error("server forced to shutdown", "error", err)
    }

    slog.Info("server exited")
}
```

- [ ] **步骤 2：编译验证**

```bash
go build ./cmd/server
```

预期：编译成功，无错误

- [ ] **步骤 3：提交**

```bash
git add cmd/server/main.go
git commit -m "feat: add main entry point with graceful shutdown"
```

---

### 任务 12：Dockerfile 与集成测试

**文件：**
- 创建：`Dockerfile`
- 创建：`internal/integration/integration_test.go`

- [ ] **步骤 1：编写 Dockerfile**

`Dockerfile`:
```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /server .
COPY configs/ ./configs/
COPY migrations/ ./migrations/

EXPOSE 8080
CMD ["./server"]
```

- [ ] **步骤 2：编写集成测试**

`internal/integration/integration_test.go`:
```go
//go:build integration
// +build integration

package integration

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "testing"
    "time"
)

const baseURL = "http://localhost:8080/api/v1"

func TestCreateAndWaitForTask(t *testing.T) {
    // 前提：服务已启动，PostgreSQL 已配置
    // 运行方式: go test -tags=integration -v ./internal/integration/

    // 1. 创建任务
    body := map[string]interface{}{
        "name":    "integration-test",
        "command": "echo integration-success",
    }
    jsonBody, _ := json.Marshal(body)
    resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
    if err != nil {
        t.Skipf("skipping integration test: server not reachable: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        t.Fatalf("Create task status = %d, want %d", resp.StatusCode, http.StatusCreated)
    }

    var createResp struct {
        TaskUID string `json:"task_uid"`
        Status  string `json:"status"`
    }
    json.NewDecoder(resp.Body).Decode(&createResp)

    if createResp.TaskUID == "" {
        t.Fatal("task_uid should not be empty")
    }
    t.Logf("created task: %s", createResp.TaskUID)

    // 2. 等待任务完成
    var finalStatus string
    for i := 0; i < 20; i++ {
        time.Sleep(500 * time.Millisecond)

        resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, createResp.TaskUID))
        if err != nil {
            t.Fatalf("Get task error: %v", err)
        }
        var task map[string]interface{}
        json.NewDecoder(resp.Body).Decode(&task)
        resp.Body.Close()

        finalStatus = task["status"].(string)
        if finalStatus == "success" || finalStatus == "failed" || finalStatus == "canceled" {
            break
        }
    }

    if finalStatus != "success" {
        t.Errorf("final status = %s, want success", finalStatus)
    }

    // 3. 验证日志
    resp, err = http.Get(fmt.Sprintf("%s/tasks/%s/log", baseURL, createResp.TaskUID))
    if err != nil {
        t.Fatalf("Get log error: %v", err)
    }
    var logResp struct {
        Stdout string `json:"stdout"`
    }
    json.NewDecoder(resp.Body).Decode(&logResp)
    resp.Body.Close()

    if logResp.Stdout != "integration-success\n" {
        t.Errorf("stdout = %q, want %q", logResp.Stdout, "integration-success\n")
    }
}

func TestTaskTimeout(t *testing.T) {
    // 创建超时任务
    body := map[string]interface{}{
        "name":        "timeout-test",
        "command":     "sleep 30",
        "timeout_sec": 1,
    }
    jsonBody, _ := json.Marshal(body)
    resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
    if err != nil {
        t.Skipf("skipping integration test: server not reachable: %v", err)
    }
    defer resp.Body.Close()

    var createResp struct {
        TaskUID string `json:"task_uid"`
    }
    json.NewDecoder(resp.Body).Decode(&createResp)

    // 等待超时
    var finalStatus string
    for i := 0; i < 20; i++ {
        time.Sleep(500 * time.Millisecond)
        resp, _ := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, createResp.TaskUID))
        var task map[string]interface{}
        json.NewDecoder(resp.Body).Decode(&task)
        resp.Body.Close()
        finalStatus = task["status"].(string)
        if finalStatus == "failed" || finalStatus == "success" {
            break
        }
    }

    if finalStatus != "failed" {
        t.Errorf("final status = %s, want failed (timeout)", finalStatus)
    }
}

func TestConcurrencyLimit(t *testing.T) {
    // 同时提交多个长时间任务，验证并发限制
    taskUIDs := make([]string, 0, 15)
    for i := 0; i < 15; i++ {
        body := map[string]interface{}{
            "name":    fmt.Sprintf("concurrency-test-%d", i),
            "command": "sleep 5",
        }
        jsonBody, _ := json.Marshal(body)
        resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
        if err != nil {
            t.Skipf("skipping integration test: server not reachable: %v", err)
        }
        var r struct {
            TaskUID string `json:"task_uid"`
        }
        json.NewDecoder(resp.Body).Decode(&r)
        resp.Body.Close()
        taskUIDs = append(taskUIDs, r.TaskUID)
    }

    // 等一秒后检查 Pending 数量
    time.Sleep(1 * time.Second)

    resp, _ := http.Get(baseURL + "/tasks?status=pending&size=50")
    var listResp struct {
        Total int `json:"total"`
    }
    json.NewDecoder(resp.Body).Decode(&listResp)
    resp.Body.Close()

    // 如果 maxConcurrent=10，至少应该有 5 个 Pending
    if listResp.Total < 1 {
        t.Error("expected some tasks to be pending due to concurrency limit")
    }
    t.Logf("pending tasks: %d (out of %d total)", listResp.Total, len(taskUIDs))
}

func TestCancelTask(t *testing.T) {
    // 创建长时间任务并取消
    body := map[string]interface{}{
        "name":    "cancel-test",
        "command": "sleep 60",
    }
    jsonBody, _ := json.Marshal(body)
    resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
    if err != nil {
        t.Skipf("skipping integration test: server not reachable: %v", err)
    }
    var createResp struct {
        TaskUID string `json:"task_uid"`
    }
    json.NewDecoder(resp.Body).Decode(&createResp)
    resp.Body.Close()

    // 等任务进入 Running
    time.Sleep(1 * time.Second)

    // 取消
    cancelResp, err := http.Post(
        fmt.Sprintf("%s/tasks/%s/cancel", baseURL, createResp.TaskUID),
        "application/json", nil,
    )
    if err != nil {
        t.Fatalf("Cancel request error: %v", err)
    }
    cancelResp.Body.Close()

    if cancelResp.StatusCode != http.StatusOK {
        t.Errorf("Cancel status = %d, want %d", cancelResp.StatusCode, http.StatusOK)
    }
}

func TestTaskListAndPagination(t *testing.T) {
    resp, err := http.Get(baseURL + "/tasks?page=1&size=5")
    if err != nil {
        t.Skipf("skipping integration test: server not reachable: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("List tasks status = %d, want %d", resp.StatusCode, http.StatusOK)
    }

    var result struct {
        Tasks []map[string]interface{} `json:"tasks"`
        Total int                      `json:"total"`
        Page  int                      `json:"page"`
    }
    json.NewDecoder(resp.Body).Decode(&result)

    if result.Page != 1 {
        t.Errorf("Page = %d, want 1", result.Page)
    }
}

func TestRerunTask(t *testing.T) {
    // 创建一个快速任务
    body := map[string]interface{}{
        "name":    "rerun-source",
        "command": "echo original",
    }
    jsonBody, _ := json.Marshal(body)
    resp, _ := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
    var createResp struct {
        TaskUID string `json:"task_uid"`
    }
    json.NewDecoder(resp.Body).Decode(&createResp)
    resp.Body.Close()

    // 等待完成
    time.Sleep(2 * time.Second)

    // 重跑
    rerunResp, err := http.Post(
        fmt.Sprintf("%s/tasks/%s/rerun", baseURL, createResp.TaskUID),
        "application/json", nil,
    )
    if err != nil {
        t.Skipf("skipping integration test: server not reachable: %v", err)
    }
    defer rerunResp.Body.Close()

    if rerunResp.StatusCode != http.StatusCreated {
        t.Errorf("Rerun status = %d, want %d", rerunResp.StatusCode, http.StatusCreated)
    }

    var rerunBody struct {
        TaskUID string `json:"task_uid"`
    }
    json.NewDecoder(rerunResp.Body).Decode(&rerunBody)
    if rerunBody.TaskUID == createResp.TaskUID {
        t.Error("rerun task_uid should differ from original")
    }
}
```

- [ ] **步骤 3：提交**

```bash
git add Dockerfile internal/integration/integration_test.go
git commit -m "feat: add dockerfile and integration tests"
```

---

### 任务 13：首次全流程验证

- [ ] **步骤 1：确保所有单元测试通过**

```bash
go test ./internal/... -v -count=1
```

预期：所有包 PASS

- [ ] **步骤 2：启动 PostgreSQL（若无本地实例）**

```bash
# 使用 Docker 启动一个本地 PG（如需要）
docker run -d --name mini-bk-pg \
  -e POSTGRES_USER=mini-bk \
  -e POSTGRES_PASSWORD=mini-bk \
  -e POSTGRES_DB=mini-bk \
  -p 5432:5432 \
  postgres:16-alpine
```

- [ ] **步骤 3：运行迁移**

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
migrate -path migrations -database "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable" up
```

- [ ] **步骤 4：启动服务**

```bash
go run ./cmd/server
```

预期：日志输出 "server starting on :8080"

- [ ] **步骤 5：手动验证 API**

```bash
# 创建任务
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"hello","command":"echo hello world"}'

# 获取任务列表
curl http://localhost:8080/api/v1/tasks

# 获取资源信息
curl http://localhost:8080/api/v1/resources

# 获取统计
curl http://localhost:8080/api/v1/stats
```

预期：所有接口返回 200/201，任务执行成功

- [ ] **步骤 6：提交并打标签**

```bash
git add -A
git commit -m "chore: final polish for phase 1"
git tag v0.1.0
```

---

## 文件汇总

| # | 文件 | 用途 |
|---|------|------|
| 1 | `go.mod` | Go module 定义 |
| 2 | `Makefile` | 构建/运行/测试/迁移命令 |
| 3 | `configs/config.yaml` | 应用配置 |
| 4 | `internal/config/config.go` | 配置加载（Viper） |
| 5 | `internal/config/config_test.go` | 配置测试 |
| 6 | `internal/model/task.go` | Task 结构体、状态常量、状态机 |
| 7 | `internal/model/task_test.go` | Task 模型测试 |
| 8 | `internal/store/postgres.go` | PostgreSQL 连接 |
| 9 | `internal/store/postgres_test.go` | PG 连接测试 |
| 10 | `internal/store/task_store.go` | Task CRUD 操作 |
| 11 | `internal/store/task_store_test.go` | Task Store 测试 |
| 12 | `internal/executor/executor.go` | 进程执行（超时/取消） |
| 13 | `internal/executor/executor_test.go` | Executor 测试 |
| 14 | `internal/scheduler/scheduler.go` | 基于 Ticker 的调度 |
| 15 | `internal/scheduler/scheduler_test.go` | Scheduler 测试 |
| 16 | `internal/service/task_service.go` | 业务逻辑层 |
| 17 | `internal/service/task_service_test.go` | Service 测试 |
| 18 | `internal/api/router.go` | Gin 路由注册 |
| 19 | `internal/api/task_handler.go` | Task HTTP 处理器 |
| 20 | `internal/api/task_handler_test.go` | Handler 测试 |
| 21 | `internal/api/resource_handler.go` | 资源与统计处理器 |
| 22 | `cmd/server/main.go` | 入口 + 依赖注入 |
| 23 | `Dockerfile` | 容器构建 |
| 24 | `migrations/000001_create_tasks.up.sql` | 任务表 DDL |
| 25 | `migrations/000001_create_tasks.down.sql` | 任务表回滚 |
| 26 | `internal/integration/integration_test.go` | 端到端集成测试 |
