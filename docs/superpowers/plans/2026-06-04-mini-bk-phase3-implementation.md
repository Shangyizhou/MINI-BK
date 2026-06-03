# Mini-BK ResourceOps 三期实现计划

> **给执行者的说明：** 必须使用 superpowers:subagent-driven-development 按任务逐个实现。步骤使用 checkbox（`- [ ]`）跟踪进度。

**目标：** 任务在多台机器上执行，Server 根据节点资源智能调度。引入 gRPC + Agent 二进制。

**架构：** Server 侧新增 gRPC Server + NodeManager + AgentRegistry；新增独立 Agent 二进制（gRPC Client + 本地 Executor + 心跳 Loop）。

**技术栈：** Go 1.22+, Gin, PostgreSQL, Redis, gRPC + Protobuf

**设计文档：** `docs/superpowers/specs/2026-06-01-mini-bk-resourceops-design.md`（§4 三期）

---

### 任务 1: Proto 定义 + 代码生成

**涉及文件：**
- 新建: `pkg/proto/agent.proto`
- 新建: `pkg/proto/`（生成的 .pb.go 文件）

- [ ] **Step 1: 安装 protoc 和 Go 插件**
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

- [ ] **Step 2: 编写 proto 文件**

```protobuf
syntax = "proto3";
package agent;
option go_package = "github.com/shangyizhou/mini-bk/pkg/proto";

service AgentService {
    rpc Register(RegisterRequest) returns (RegisterResponse);
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
    rpc SubmitTask(TaskRequest) returns (TaskResponse);
    rpc CancelTask(CancelRequest) returns (CancelResponse);
    rpc StreamLog(stream LogChunk) returns (stream ServerMessage);
    rpc ReportResult(ResultRequest) returns (ResultResponse);
}

message RegisterRequest {
    string hostname = 1;
    string ip = 2;
    string version = 3;
    int64 total_cpu = 4;
    int64 total_memory_mb = 5;
    int64 total_disk_mb = 6;
    repeated string labels = 7;
}

message RegisterResponse {
    string node_id = 1;
    bool accepted = 2;
    string message = 3;
}

message NodeResource {
    double cpu_usage_percent = 1;
    int64 memory_used_mb = 2;
    int64 memory_total_mb = 3;
    int64 disk_used_mb = 4;
    int64 disk_total_mb = 5;
    double load_avg_1m = 6;
    int32 running_tasks = 7;
}

message HeartbeatRequest {
    string node_id = 1;
    string version = 2;
    NodeResource resources = 3;
}

message HeartbeatResponse {
    bool ok = 1;
}

message TaskRequest {
    string task_uid = 1;
    string name = 2;
    string command = 3;
    string workdir = 4;
    map<string, string> env = 5;
    int32 timeout_sec = 6;
}

message TaskResponse {
    bool accepted = 1;
}

message CancelRequest {
    string task_uid = 1;
}

message CancelResponse {
    bool ok = 1;
}

message LogChunk {
    string task_uid = 1;
    string line = 2;
    string stream = 3; // "stdout" or "stderr"
    int64 timestamp = 4;
}

message ServerMessage {
    string task_uid = 1;
    string action = 2; // "cancel"
}

message ResultRequest {
    string task_uid = 1;
    int32 exit_code = 2;
    string stdout = 3;
    string stderr = 4;
    string error_message = 5;
    bool timed_out = 6;
}

message ResultResponse {
    bool ok = 1;
}
```

- [ ] **Step 3: 生成 Go 代码**
```bash
protoc --go_out=. --go-grpc_out=. pkg/proto/agent.proto
```

- [ ] **Step 4: 提交** `feat: 添加 gRPC proto 定义和生成代码`

---

### 任务 2: Node 模型 + 节点持久化

**涉及文件：**
- 新建: `internal/model/node.go`
- 新建: `migrations/000003_create_nodes.up.sql`
- 修改: `internal/store/node_store.go`

- [ ] **Step 1: 迁移脚本**

```sql
CREATE TABLE nodes (
    id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(36) NOT NULL UNIQUE,
    hostname VARCHAR(255) NOT NULL,
    ip VARCHAR(45) NOT NULL,
    version VARCHAR(20) DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'offline', -- online/offline/drain/cordon
    total_cpu INT DEFAULT 0,
    total_memory_mb INT DEFAULT 0,
    total_disk_mb INT DEFAULT 0,
    cpu_usage_percent DOUBLE PRECISION DEFAULT 0,
    memory_used_mb INT DEFAULT 0,
    disk_used_mb INT DEFAULT 0,
    load_avg_1m DOUBLE PRECISION DEFAULT 0,
    running_tasks INT DEFAULT 0,
    labels TEXT[] DEFAULT '{}',
    last_heartbeat_at TIMESTAMPTZ,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nodes_status ON nodes(status);
CREATE INDEX idx_nodes_labels ON nodes USING GIN(labels);
```

- [ ] **Step 2: Node 模型**

```go
type NodeStatus string
const (
    NodeStatusOnline  NodeStatus = "online"
    NodeStatusOffline NodeStatus = "offline"
    NodeStatusDrain   NodeStatus = "drain"
    NodeStatusCordon  NodeStatus = "cordon"
)

type Node struct {
    ID              int64      `json:"id"`
    NodeID          string     `json:"node_id"`
    Hostname        string     `json:"hostname"`
    IP              string     `json:"ip"`
    Version         string     `json:"version"`
    Status          NodeStatus `json:"status"`
    TotalCPU        int        `json:"total_cpu"`
    TotalMemoryMB   int        `json:"total_memory_mb"`
    TotalDiskMB     int        `json:"total_disk_mb"`
    CPUUsagePct     float64    `json:"cpu_usage_percent"`
    MemoryUsedMB    int        `json:"memory_used_mb"`
    DiskUsedMB      int        `json:"disk_used_mb"`
    LoadAvg1m       float64    `json:"load_avg_1m"`
    RunningTasks    int        `json:"running_tasks"`
    Labels          []string   `json:"labels"`
    LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
    RegisteredAt    time.Time  `json:"registered_at"`
    // ...
}

func (n *Node) AllocatedRatio() float64 { ... }
func (n *Node) IsSchedulable() bool { ... }
func (n *Node) HasLabel(label string) bool { ... }
func (n *Node) MatchLabels(selector map[string]string) bool { ... }
```

- [ ] **Step 3: NodeStore** CRUD + ByStatus + ByLabels + heartbeat update

- [ ] **Step 4: 提交** `feat: 添加 Node 模型和节点持久化`

---

### 任务 3: Agent 二进制（gRPC 客户端 + 心跳 + 执行）

**涉及文件：**
- 新建: `cmd/agent/main.go`

Agent 核心循环：
1. 启动时收集本机信息（hostname, IP, CPU cores, memory, disk via gopsutil）
2. 连接 Server gRPC → 调用 Register
3. 启动心跳 goroutine（每 10s 发送资源快照）
4. 启动任务接收 goroutine（监听 Server 下发的 SubmitTask）
5. 本地执行任务（复用 executor 包的核心逻辑）
6. 通过 StreamLog 双向流回传日志
7. 任务完成后调用 ReportResult

```go
// cmd/agent/main.go 关键结构
type Agent struct {
    nodeID      string
    hostname    string
    ip          string
    version     string
    labels      []string
    serverAddr  string
    grpcConn    *grpc.ClientConn
    client      proto.AgentServiceClient
    executor    *executor.LocalExecutor // 本地进程执行
    running     map[string]context.CancelFunc // 运行中的任务
    mu          sync.Mutex
}
```

启动参数：`-server-addr=localhost:50051 -labels=linux,test`

- [ ] **Step: TDD（mock gRPC server）→ 实现 → 提交** `feat: 添加 Agent 二进制`

---

### 任务 4: Server 侧 gRPC 服务 + NodeManager

**涉及文件：**
- 新建: `internal/grpcserver/server.go`
- 新建: `internal/nodemanager/manager.go`

NodeManager 职责：
- 处理 Register：分配 node_id（UUID），写入 PostgreSQL + Redis 缓存
- 处理 Heartbeat：更新 last_heartbeat_at + 资源快照
- 后台检测离线：每 5s 检查 last_heartbeat_at > 30s → 标记 OFFLINE
- 节点查询：ByLabels, Online, All

gRPC Server 实现 AgentService 接口，委托给 NodeManager + Scheduler。

- [ ] **Step: TDD → 实现 → 提交** `feat: 添加 Server 侧 gRPC 服务和节点管理`

---

### 任务 5: 重构 Scheduler（多节点调度）

**涉及文件：**
- 修改: `internal/scheduler/scheduler.go`

核心变更：`SelectNode(task) (*Node, error)` 三层过滤
1. 标签匹配（task.NodeSelector → NodeManager.FindByLabels）
2. 资源过滤（CPU + Memory 余量检查）
3. LeastAllocated 打分排序

dispatch 改为：SelectNode → gRPC SubmitTask 到目标 Agent → Agent 本地执行 → StreamLog 回传 → ReportResult

本地 Executor 保留作为 fallback（开发/测试时不用启动 Agent）。

- [ ] **Step: TDD → 实现 → 提交** `feat: 重构调度器支持多节点三层过滤调度`

---

### 任务 6: gRPC 客户端封装（Server → Agent 通信）

**涉及文件：**
- 新建: `internal/grpcclient/client.go`

```go
type AgentClient struct {
    conn   *grpc.ClientConn
    client proto.AgentServiceClient
}

func (c *AgentClient) SubmitTask(ctx, task) error { ... }
func (c *AgentClient) CancelTask(ctx, taskUID) error { ... }
func (c *AgentClient) StreamLog(ctx, taskUID) (proto.AgentService_StreamLogClient, error) { ... }
```

Scheduler 通过 AgentClient 与远程 Agent 通信。

- [ ] **Step: 提交** `feat: 添加 gRPC 客户端封装`

---

### 任务 7: Executor 重构（本地 vs 远程）

**涉及文件：**
- 修改: `internal/executor/executor.go`

将 Executor 接口化：
```go
type TaskExecutor interface {
    Run(ctx context.Context, task *model.Task) *TaskResult
}
```

保留 `LocalExecutor`（Phase 1/2 的实现），Scheduler 内部根据是否有可用远程节点选择本地或远程执行。

- [ ] **Step: 提交** `feat: 执行器接口化，支持本地/远程双模式`

---

### 任务 8: Agent 任务执行 + 日志回传**

**涉及文件：**
- 修改: `cmd/agent/main.go`
- 修改: `internal/executor/executor.go`

Agent 收到 SubmitTask 后：
1. 创建工作目录 `<workdir>/tasks/<task_uid>/`
2. 启动 LocalExecutor.Run() 在 goroutine 中执行
3. stdout/stderr 通过 StreamLog 双向流推送到 Server
4. Server 侧接收 StreamLog → 写入 LogStream（Redis Stream）
5. 执行完成后调用 ReportResult
6. 监听 ServerMessage 中的 cancel action → kill 进程

- [ ] **Step: 提交** `feat: Agent 任务执行和日志双向流回传`

---

### 任务 9: 节点管理 API

**涉及文件：**
- 修改: `internal/api/router.go`
- 新建: `internal/api/node_handler.go`

新增端点：
```
GET    /api/v1/nodes              节点列表
GET    /api/v1/nodes/:node_id     节点详情
POST   /api/v1/nodes/:node_id/drain    禁止调度
POST   /api/v1/nodes/:node_id/uncordon 恢复调度
```

- [ ] **Step: 提交** `feat: 添加节点管理 API`

---

### 任务 10: Task 模型扩展（节点选择器）

**涉及文件：**
- 修改: `internal/model/task.go`
- 修改: `migrations/000004_add_node_selector.up.sql`

```sql
ALTER TABLE tasks ADD COLUMN node_selector JSONB DEFAULT '{}';
ALTER TABLE tasks ADD COLUMN assigned_node_id VARCHAR(36);
```

```go
// Task 新增字段
NodeSelector  map[string]string `json:"node_selector"` // {"gpu": "true", "env": "prod"}
AssignedNodeID string           `json:"assigned_node_id"`
```

Store Update/Create 同步更新。

- [ ] **Step: 提交** `feat: 任务模型新增节点选择器和分配字段`

---

### 任务 11: main.go + Agent 启动脚本

**涉及文件：**
- 修改: `cmd/server/main.go`
- 修改: `cmd/agent/main.go`

main.go: 初始化 gRPC Server + NodeManager → 在单独端口启动 gRPC（如 :50051）
Agent: 读取命令行参数 → 连接 Server → Register → 启动心跳 + 任务接收循环

- [ ] **Step: 提交** `feat: 主入口集成 gRPC 服务和 Agent 启动脚本`

---

### 任务 12: Docker Compose + 集成测试 + 文档

- 更新 `deployments/docker-compose.yml` 添加 Agent 服务
- 新建 Phase 3 集成测试（agent 注册、心跳、远程执行、标签调度、drain/cordon）
- 更新 `docs/context.md` 和 `docs/history.md`
- 更新 `docs/features/` 添加 Phase 3 proposal
- 打标签 `v0.3.0`
