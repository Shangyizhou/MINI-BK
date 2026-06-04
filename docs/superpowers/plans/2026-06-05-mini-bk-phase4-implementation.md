# Mini-BK ResourceOps 四期实现计划

> **给执行者的说明：** 必须使用 superpowers:subagent-driven-development 按任务逐个实现。

**目标：** 消除 Scheduler 单点，实现多 Scheduler 高可用调度。引入 etcd 作为控制面一致性存储。

**架构：** NodeManager 的注册/心跳从 PostgreSQL+Redis 迁移到 etcd Lease；Scheduler 通过 etcd Leader Election 选主；任务分配用 etcd CAS 防重复抢占。

**技术栈：** Go 1.22+, Gin, PostgreSQL, Redis, gRPC, etcd(clientv3)

**设计文档：** `docs/superpowers/specs/2026-06-01-mini-bk-resourceops-design.md`（§5 四期）

---

### 任务 1: etcd 连接与配置

**涉及文件：**
- 修改: `configs/config.yaml`（新增 etcd 配置段）
- 修改: `internal/config/config.go`
- 新建: `internal/store/etcd.go`

etcd 配置：
```yaml
etcd:
  endpoints:
    - "localhost:2379"
  dial_timeout_sec: 5
```

`internal/store/etcd.go`:
```go
type EtcdStore struct {
    Client *clientv3.Client
}

func NewEtcdStore(ctx context.Context, endpoints []string, dialTimeout time.Duration) (*EtcdStore, error) {
    cli, err := clientv3.New(clientv3.Config{
        Endpoints:   endpoints,
        DialTimeout: dialTimeout,
    })
    if err != nil { return nil, err }
    return &EtcdStore{Client: cli}, nil
}
```

- [ ] 安装 `go get go.etcd.io/etcd/client/v3`
- [ ] 提交: `feat: 添加 etcd 连接管理和配置`

---

### 任务 2: Leader Election

**涉及文件：**
- 新建: `internal/election/election.go`

```go
type LeaderElection struct {
    client   *clientv3.Client
    session  *concurrency.Session
    election *concurrency.Election
    isLeader atomic.Bool
}

func NewLeaderElection(client *clientv3.Client, prefix string) (*LeaderElection, error)
func (le *LeaderElection) Campaign(ctx context.Context) error   // 阻塞直到成为 Leader 或 ctx 取消
func (le *LeaderElection) IsLeader() bool
func (le *LeaderElection) Resign(ctx context.Context) error      // 主动放弃
func (le *LeaderElection) Observe(ctx context.Context) <-chan bool // 监听 Leader 变更
```

使用 etcd concurrency 包的 Election + Session：
- Session 带 TTL（如 10s），定期 KeepAlive
- Campaign 当前节点成为 Leader
- Session 过期 → etcd 自动释放 → 其他节点竞选

- [ ] 提交: `feat: 添加 etcd Leader Election 模块`

---

### 任务 3: NodeManager 迁移到 etcd

**涉及文件：**
- 修改: `internal/nodemanager/manager.go`

核心变更：
- Register: Agent 注册时，使用 etcd Lease（TTL=30s）创建 key `/nodes/<node_id>/status`，value 为节点信息的 JSON
- Heartbeat: Agent 心跳时续约 Lease（KeepAliveOnce）
- 离线检测: etcd Watch `/nodes/` 前缀，当 key 过期自动删除时触发 delete 事件 → 标记节点 OFFLINE
- 不再依赖 NodeStore PostgreSQL 的 heartbeat 更新
- PostgreSQL 保留作为节点注册的持久化记录（Create on Register）
- Redis 缓存节点信息用于快速查询（可选，etcd Watch 直接提供）

简化：Register 时写 PostgreSQL（持久记录）+ etcd（存活状态）；Heartbeat 续约 etcd Lease；离线检测靠 etcd Watch Delete 事件。

- [ ] 提交: `feat: NodeManager 迁移到 etcd Lease 存活检测`

---

### 任务 4: 调度防重复抢占（CAS）

**涉及文件：**
- 修改: `internal/scheduler/scheduler.go`

在 dispatch 前增加 ClaimTask：
```go
func (s *Scheduler) ClaimTask(ctx context.Context, taskUID string) (bool, error) {
    key := fmt.Sprintf("/tasks/claimed/%s", taskUID)
    txn := s.etcd.Txn(ctx).
        If(clientv3.Compare(clientv3.Version(key), "=", 0)).
        Then(clientv3.OpPut(key, s.schedulerID, clientv3.WithLease(s.leaseID))).
        Else(clientv3.OpGet(key))
    resp, err := txn.Commit()
    if err != nil { return false, err }
    return resp.Succeeded, nil
}
```

Scheduler tick 流程更新：
1. (仅 Leader 执行调度)
2. Pop 任务 from Queue
3. ClaimTask CAS → 成功则 dispatch，失败则跳过（已被其他 Scheduler 抢占）
4. Claim 的 key 带 Lease，Scheduler 崩溃时 Lease 过期自动释放

Scheduler Start 流程：
1. 创建 etcd Session + Lease
2. 启动 LeaderElection.Campaign()
3. 成为 Leader 后启动 tick loop
4. 失去 Leader 时停止 tick，重新竞选

- [ ] 提交: `feat: 添加 etcd CAS 防重复调度和 Leader 调度`

---

### 任务 5: 服务发现

**涉及文件：**
- 修改: `internal/nodemanager/manager.go`
- 新建: `internal/discovery/discovery.go`

API Server 通过 etcd Watch 获取实时节点列表，无需轮询数据库。

```go
type ServiceDiscovery struct {
    client     *clientv3.Client
    nodesBySvc map[string][]*model.Node
    watchers   map[string]clientv3.WatchChan
}

func (sd *ServiceDiscovery) WatchNodes(ctx context.Context) <-chan NodeEvent { ... }
func (sd *ServiceDiscovery) GetServiceEndpoints(ctx, serviceName string) ([]string, error) { ... }
```

Phase 4 先聚焦节点发现：Watch `/nodes/` 前缀，实时更新内存中的节点缓存。

- [ ] 提交: `feat: 添加 etcd Watch 服务发现`

---

### 任务 6: Scheduler 集成 Leader Election

**涉及文件：**
- 修改: `cmd/server/main.go`
- 修改: `internal/scheduler/scheduler.go`

main.go:
```go
// 创建 etcd 连接
etcdStore := store.NewEtcdStore(...)

// 创建 Leader Election
election := election.NewLeaderElection(etcdStore.Client, "/election/scheduler/")

// 创建 Scheduler（注入 etcd, election, schedulerID）
sched := scheduler.NewScheduler(..., etcdStore.Client, election, schedulerID)

// 启动调度（内部处理选举）
go sched.Start(schedCtx)
```

Scheduler.Start 改为：
```go
func (s *Scheduler) Start(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            slog.Info("等待成为 Leader...")
            if err := s.election.Campaign(ctx); err != nil {
                continue
            }
            slog.Info("已成为 Leader，开始调度")
            s.runAsLeader(ctx)  // 阻塞直到失去 Leader
            slog.Warn("失去 Leader 身份")
        }
    }
}
```

- [ ] 提交: `feat: Scheduler 集成 Leader Election`

---

### 任务 7: 配置热更新（etcd Watch）

**涉及文件：**
- 新建: `internal/config/watcher.go`

```go
type ConfigWatcher struct {
    client *clientv3.Client
}

func (cw *ConfigWatcher) WatchConfig(ctx context.Context, key string, callback func([]byte)) { ... }
```

Scheduler 配置（max_concurrent, tick_interval）可通过 etcd `/config/scheduler/` 动态更新。

- [ ] 提交: `feat: 添加 etcd 配置热更新`

---

### 任务 8: main.go 集成 + Docker Compose

**涉及文件：**
- 修改: `cmd/server/main.go`
- 修改: `deployments/docker-compose.yml`

docker-compose.yml 添加 etcd 服务：
```yaml
etcd:
  image: quay.io/coreos/etcd:v3.5
  command: etcd -listen-client-urls http://0.0.0.0:2379 -advertise-client-urls http://etcd:2379
```

main.go: 
- 初始化 etcd → 创建 LeaderElection + ServiceDiscovery + ConfigWatcher
- Scheduler 改为 Leader-aware 模式
- 可启动多个 server 实例（通过 docker-compose scale）

- [ ] 提交: `feat: 主入口集成 etcd 和 Docker Compose 多实例`

---

### 任务 9: 集成测试 + 文档 + 标签

- 新增 etcd 集成测试（Leader Election、CAS、Watch）
- 更新 `docs/context.md`：etcd 技术栈 ✅，三层存储架构图
- 更新 `docs/history.md`：Phase 4 记录
- 创建 `docs/features/2026-06-05-mini-bk-phase4/proposal.md`
- `go test ./... && go build ./cmd/server && go build ./cmd/agent`
- 打标签 `v0.4.0`
