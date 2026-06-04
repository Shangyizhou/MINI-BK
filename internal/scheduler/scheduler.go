package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/shangyizhou/mini-bk/internal/config"
	"github.com/shangyizhou/mini-bk/internal/election"
	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/queue"
)

// TaskStore defines the interface for task persistence operations used by the scheduler.
type TaskStore interface {
	GetCreatedTasks(ctx context.Context) ([]*model.Task, error)
	GetPendingTasks(ctx context.Context) ([]*model.Task, error)
	GetRunningTasks(ctx context.Context) ([]*model.Task, error)
	Update(ctx context.Context, task *model.Task) error
	GetByUID(ctx context.Context, uid string) (*model.Task, error)
}

// NodeManager defines the interface for node management used by the scheduler.
type NodeManager interface {
	FindByLabels(selector map[string]string) []*model.Node
}

const maxRetryDelay = 5 * time.Minute

// Scheduler periodically checks for tasks to schedule and dispatches them
// based on available resources.
type Scheduler struct {
	store         TaskStore
	executor      executor.TaskExecutor
	nodeMgr       NodeManager
	tickInterval  time.Duration
	maxConcurrent int
	totalCPU      int
	totalMemMB    int
	rdb           *redis.Client
	cancelFuncs   map[string]context.CancelFunc // taskUID -> cancel
	cancelMu      sync.Mutex
	queue         queue.TaskQueue
	nodeTasks     map[string]chan *model.Task // nodeID -> pending task queue
	nodeTasksMu   sync.Mutex
	etcdClient    *clientv3.Client  // etcd client for CAS claims
	schedulerID   string            // unique ID for this scheduler instance
	leaseID       clientv3.LeaseID  // etcd lease for claim keys
	election      *election.LeaderElection // optional leader election (added in Phase 4 Task 6)
	configWatcher *config.ConfigWatcher    // config hot reload via etcd Watch (added in Phase 4 Task 7)
}

// NewScheduler creates a new Scheduler with the given store, executor, tick interval,
// and max concurrency. It detects total CPU via runtime.NumCPU() and defaults
// totalMemMB to 8192. If etcdClient is provided, it uses etcd CAS for duplicate
// scheduling prevention. If election is provided, it runs with leader election.
func NewScheduler(store TaskStore, exec executor.TaskExecutor, tickInterval time.Duration, maxConcurrent int, rdb *redis.Client, q queue.TaskQueue, etcdClient *clientv3.Client, leaseID clientv3.LeaseID, election *election.LeaderElection) *Scheduler {
	schedulerID := fmt.Sprintf("scheduler-%s", uuid.New().String()[:8])
	return &Scheduler{
		store:         store,
		executor:      exec,
		tickInterval:  tickInterval,
		maxConcurrent: maxConcurrent,
		totalCPU:      runtime.NumCPU(),
		totalMemMB:    8192,
		rdb:           rdb,
		cancelFuncs:   make(map[string]context.CancelFunc),
		queue:         q,
		nodeTasks:     make(map[string]chan *model.Task),
		etcdClient:     etcdClient,
		schedulerID:    schedulerID,
		leaseID:        leaseID,
		election:       election,
		configWatcher: func() *config.ConfigWatcher {
			if etcdClient != nil {
				return config.NewConfigWatcher(etcdClient)
			}
			return nil
		}(),
	}
}

// SetNodeManager sets the NodeManager for multi-node scheduling.
// If not set, the scheduler operates in local-only mode.
func (s *Scheduler) SetNodeManager(nm NodeManager) {
	s.nodeMgr = nm
}

// Start starts the scheduler. If LeaderElection is configured, only dispatches
// when leader. It also listens for task cancellation via Redis Pub/Sub.
func (s *Scheduler) Start(ctx context.Context) {
	// Start Redis Pub/Sub listener for task cancellation (runs regardless of leader status)
	if s.rdb != nil {
		go func() {
			pubsub := s.rdb.PSubscribe(ctx, "tasks:cancel:*")
			defer pubsub.Close()

			// Wait for subscription to be ready
			_, err := pubsub.Receive(ctx)
			if err != nil {
				slog.Error("scheduler: failed to subscribe to cancel channel", "error", err)
				return
			}

			slog.Info("scheduler: listening for task cancellation via Redis Pub/Sub")
			ch := pubsub.Channel()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						return
					}
					taskUID := strings.TrimPrefix(msg.Channel, "tasks:cancel:")
					s.cancelMu.Lock()
					if cancel, ok := s.cancelFuncs[taskUID]; ok {
						slog.Info("scheduler: cancelling task via Pub/Sub", "task_uid", taskUID)
						cancel()
						delete(s.cancelFuncs, taskUID)
					}
					s.cancelMu.Unlock()
				}
			}
		}()
	}

	// Start etcd config watcher for dynamic config changes (Task 7)
	if s.configWatcher != nil {
		s.configWatcher.Watch(ctx, "/config/scheduler/", func(key string, value []byte) {
			switch {
			case strings.HasSuffix(key, "/max_concurrent_tasks"):
				var val int
				fmt.Sscanf(string(value), "%d", &val)
				if val > 0 {
					s.maxConcurrent = val
					slog.Info("调度器配置更新", "max_concurrent", val)
				}
			case strings.HasSuffix(key, "/tick_interval_ms"):
				var val int
				fmt.Sscanf(string(value), "%d", &val)
				if val > 0 {
					s.tickInterval = time.Duration(val) * time.Millisecond
					slog.Info("调度器配置更新", "tick_interval_ms", val)
				}
			}
		})
	}

	if s.election == nil {
		// No election configured, run as single scheduler (Phase 1-3 behavior)
		slog.Info("无 Leader Election，以单调度器模式启动")
		s.runTickLoop(ctx)
		return
	}

	// Run election loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				slog.Info("等待成为 Leader...")
				if err := s.election.Campaign(ctx); err != nil {
					slog.Warn("竞选失败，重试", "error", err)
					time.Sleep(2 * time.Second)
					continue
				}
				// Became leader
				slog.Info("已成为 Leader，开始调度")
				s.runTickLoop(ctx)
				slog.Warn("失去 Leader 身份")
			}
		}
	}()
}

// runTickLoop runs the scheduling tick loop. It checks leader status on each tick
// if leader election is configured.
func (s *Scheduler) runTickLoop(ctx context.Context) {
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()
	for {
		// Only process if we're still the leader
		if s.election != nil && !s.election.IsLeader() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick performs one scheduling cycle.
func (s *Scheduler) tick(ctx context.Context) {
	// Get currently running tasks and compute allocated resources
	runningTasks, err := s.store.GetRunningTasks(ctx)
	if err != nil {
		slog.Error("scheduler: failed to get running tasks", "error", err)
		return
	}

	allocatedCPU := 0
	allocatedMem := 0
	now := time.Now()

	for _, task := range runningTasks {
		allocatedCPU += task.CPULimit
		allocatedMem += task.MemoryLimit

		// Check for timed-out running tasks
		if task.StartedAt != nil && task.TimeoutSec > 0 {
			deadline := task.StartedAt.Add(time.Duration(task.TimeoutSec) * time.Second)
			if now.After(deadline) {
				s.failTask(ctx, task, "task timed out")
			}
		}
	}

	availableCPU := s.totalCPU - allocatedCPU
	availableMem := s.totalMemMB - allocatedMem
	availableSlots := s.maxConcurrent - len(runningTasks)

	if availableSlots <= 0 {
		return
	}

	// Pop tasks from queue and dispatch
	for availableSlots > 0 {
		task, err := s.queue.Pop(ctx, 100*time.Millisecond)
		if err != nil || task == nil {
			break // queue empty or timeout
		}

		if s.canAllocate(task, availableCPU, availableMem) {
			s.dispatch(ctx, task)
			availableCPU -= task.CPULimit
			availableMem -= task.MemoryLimit
			availableSlots--
		} else {
			// Not enough resources, push back with short delay
			_ = s.queue.PushDelayed(ctx, task, 1*time.Second)
		}
	}
}

// canAllocate checks if a task can be allocated given available resources.
func (s *Scheduler) canAllocate(task *model.Task, availableCPU, availableMem int) bool {
	if task.CPULimit > 0 && task.CPULimit > availableCPU {
		return false
	}
	if task.MemoryLimit > 0 && task.MemoryLimit > availableMem {
		return false
	}
	return true
}

// SelectNode picks the best node for a task using 3-layer filtering:
// Layer 1: Label matching
// Layer 2: Resource filtering
// Layer 3: LeastAllocated scoring
func (s *Scheduler) SelectNode(task *model.Task) (*model.Node, error) {
	if s.nodeMgr == nil {
		return nil, nil // no remote nodes, use local executor
	}

	// Layer 1: Label matching
	candidates := s.nodeMgr.FindByLabels(task.NodeSelector)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no node matching labels %v", task.NodeSelector)
	}

	// Layer 2: Resource filtering
	var filtered []*model.Node
	for _, node := range candidates {
		availableCPU := node.TotalCPU - int(float64(node.TotalCPU)*node.CPUUsagePct/100.0)
		availableMem := node.TotalMemoryMB - node.MemoryUsedMB
		if (task.CPULimit == 0 || task.CPULimit <= availableCPU) &&
			(task.MemoryLimit == 0 || task.MemoryLimit <= availableMem) {
			filtered = append(filtered, node)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no node with sufficient resources")
	}

	// Layer 3: LeastAllocated scoring (pick the least utilized node)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CPUUsagePct < filtered[j].CPUUsagePct
	})

	return filtered[0], nil
}

// ClaimTask tries to claim a task via etcd CAS transaction.
// Returns true if this scheduler successfully claimed the task.
// When etcd is not configured, always returns true (single scheduler mode).
func (s *Scheduler) ClaimTask(ctx context.Context, taskUID string) (bool, error) {
	if s.etcdClient == nil {
		return true, nil // no etcd, single scheduler, always claim
	}
	key := fmt.Sprintf("/tasks/claimed/%s", taskUID)
	txn := s.etcdClient.Txn(ctx).
		If(clientv3.Compare(clientv3.Version(key), "=", 0)).
		Then(clientv3.OpPut(key, s.schedulerID, clientv3.WithLease(s.leaseID))).
		Else(clientv3.OpGet(key))
	resp, err := txn.Commit()
	if err != nil {
		return false, fmt.Errorf("claim task: %w", err)
	}
	if resp.Succeeded {
		slog.Info("任务抢占成功", "task_uid", taskUID, "scheduler", s.schedulerID)
	}
	return resp.Succeeded, nil
}

// dispatch transitions a task to Running, updates the store, and launches
// execution in a goroutine. It uses remote execution when a suitable node is found.
func (s *Scheduler) dispatch(ctx context.Context, task *model.Task) {
	// Claim the task first (prevents duplicate scheduling)
	claimed, err := s.ClaimTask(ctx, task.TaskUID)
	if err != nil {
		slog.Error("任务抢占失败", "task_uid", task.TaskUID, "error", err)
		return
	}
	if !claimed {
		slog.Info("任务已被其他 Scheduler 抢占", "task_uid", task.TaskUID)
		return
	}

	// If task is in created state, first transition to pending
	if task.Status == model.TaskStatusCreated {
		if err := task.TransitionTo(model.TaskStatusPending); err != nil {
			slog.Error("scheduler: failed to transition task to pending", "error", err, "task_uid", task.TaskUID)
			return
		}
		if err := s.store.Update(ctx, task); err != nil {
			slog.Error("scheduler: failed to update task", "error", err, "task_uid", task.TaskUID)
			return
		}
	}

	// Try to select a remote node
	node, err := s.SelectNode(task)
	if err != nil {
		// No suitable remote node, use local executor as fallback
		slog.Debug("scheduler: no remote node, using local executor",
			"task_uid", task.TaskUID,
			"error", err,
		)
		s.dispatchLocal(ctx, task)
		return
	}

	if node == nil {
		// No remote nodes at all (nodeMgr is nil), use local
		s.dispatchLocal(ctx, task)
		return
	}

	// Remote execution via gRPC
	s.dispatchRemote(ctx, task, node)
}

// dispatchLocal executes the task locally (existing Phase 1/2 behavior)
func (s *Scheduler) dispatchLocal(ctx context.Context, task *model.Task) {
	// Transition to running
	if err := task.TransitionTo(model.TaskStatusRunning); err != nil {
		slog.Error("scheduler: failed to transition task to running", "error", err, "task_uid", task.TaskUID)
		return
	}

	now := time.Now()
	task.StartedAt = &now

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task", "error", err, "task_uid", task.TaskUID)
		return
	}

	slog.Info("scheduler: dispatching task locally",
		"task_uid", task.TaskUID,
		"name", task.Name,
		"cpu_limit", task.CPULimit,
		"memory_limit", task.MemoryLimit,
	)

	// Create cancellable context for this task
	taskCtx, taskCancel := context.WithCancel(ctx)
	s.cancelMu.Lock()
	s.cancelFuncs[task.TaskUID] = taskCancel
	s.cancelMu.Unlock()

	go func(t *model.Task) {
		defer func() {
			s.cancelMu.Lock()
			delete(s.cancelFuncs, t.TaskUID)
			s.cancelMu.Unlock()
		}()
		result := s.executor.Run(taskCtx, t)
		s.completeTask(ctx, t, result)
	}(task)
}

// dispatchRemote sends the task to a remote agent via PullTask polling
func (s *Scheduler) dispatchRemote(ctx context.Context, task *model.Task, node *model.Node) {
	// Transition to running
	if err := task.TransitionTo(model.TaskStatusRunning); err != nil {
		slog.Error("scheduler: failed to transition task to running", "error", err, "task_uid", task.TaskUID)
		return
	}

	task.AssignedNodeID = node.NodeID
	now := time.Now()
	task.StartedAt = &now

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task for remote dispatch", "error", err, "task_uid", task.TaskUID)
		return
	}

	slog.Info("scheduler: dispatching task to remote node",
		"task_uid", task.TaskUID,
		"name", task.Name,
		"node_id", node.NodeID,
		"node_hostname", node.Hostname,
	)

	// Create cancellable context for this task
	taskCtx, taskCancel := context.WithCancel(ctx)
	s.cancelMu.Lock()
	s.cancelFuncs[task.TaskUID] = taskCancel
	s.cancelMu.Unlock()

	// Push task to node's queue for agent to pick up via PullTask
	ch := s.getOrCreateNodeQueue(node.NodeID)
	select {
	case ch <- task:
		slog.Info("scheduler: task pushed to remote node queue",
			"task_uid", task.TaskUID,
			"node_id", node.NodeID,
		)
	default:
		slog.Warn("scheduler: remote node queue full, falling back to local execution",
			"task_uid", task.TaskUID,
			"node_id", node.NodeID,
		)
		// Fallback to local execution
		go func(t *model.Task) {
			defer func() {
				s.cancelMu.Lock()
				delete(s.cancelFuncs, t.TaskUID)
				s.cancelMu.Unlock()
			}()
			result := s.executor.Run(taskCtx, t)
			s.completeTask(ctx, t, result)
		}(task)
	}
}

// getOrCreateNodeQueue returns the task channel for the given node, creating one if needed.
func (s *Scheduler) getOrCreateNodeQueue(nodeID string) chan *model.Task {
	s.nodeTasksMu.Lock()
	defer s.nodeTasksMu.Unlock()
	ch, ok := s.nodeTasks[nodeID]
	if !ok {
		ch = make(chan *model.Task, 10) // buffer up to 10 tasks
		s.nodeTasks[nodeID] = ch
	}
	return ch
}

// GetNextTaskForNode returns the next pending task for the given node, or nil if none.
// This is called by the gRPC server when an agent polls for work.
func (s *Scheduler) GetNextTaskForNode(nodeID string) *model.Task {
	s.nodeTasksMu.Lock()
	defer s.nodeTasksMu.Unlock()
	ch, ok := s.nodeTasks[nodeID]
	if !ok {
		return nil
	}
	select {
	case task := <-ch:
		return task
	default:
		return nil
	}
}

// HandleRemoteResult processes a task result reported by a remote agent.
func (s *Scheduler) HandleRemoteResult(ctx context.Context, taskUID string, result *executor.TaskResult) {
	task, err := s.store.GetByUID(ctx, taskUID)
	if err != nil {
		slog.Error("scheduler: failed to get task for remote result",
			"task_uid", taskUID,
			"error", err,
		)
		return
	}

	s.cancelMu.Lock()
	delete(s.cancelFuncs, taskUID)
	s.cancelMu.Unlock()

	s.completeTask(ctx, task, result)
}

// completeTask handles the result of a completed task execution.
func (s *Scheduler) completeTask(ctx context.Context, task *model.Task, result *executor.TaskResult) {
	task.Stdout = result.Stdout
	task.Stderr = result.Stderr

	now := time.Now()
	task.FinishedAt = &now

	if result.Error != nil {
		if result.TimedOut {
			s.failTask(ctx, task, "task timed out")
		} else {
			s.failTask(ctx, task, result.Error.Error())
		}
		return
	}

	// Set exit code
	exitCode := result.ExitCode
	task.ExitCode = &exitCode

	if err := task.TransitionTo(model.TaskStatusSuccess); err != nil {
		slog.Error("scheduler: failed to transition task to success", "error", err, "task_uid", task.TaskUID)
		return
	}

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task on success", "error", err, "task_uid", task.TaskUID)
	}

	// 更新每日成功统计
	if s.rdb != nil {
		s.rdb.HIncrBy(ctx, "stats:daily:"+time.Now().Format("2006-01-02"), "success", 1)
	}
}

// failTask transitions a task to the Failed status with an error message.
// If the task supports retry and hasn't exhausted retries, it is re-enqueued.
func (s *Scheduler) failTask(ctx context.Context, task *model.Task, errMsg string) {
	task.ErrorMessage = errMsg

	if task.CanRetry() {
		task.RetryCount++
		task.Status = model.TaskStatusPending
		task.FinishedAt = nil

		if err := s.store.Update(ctx, task); err != nil {
			slog.Error("scheduler: failed to update task for retry", "error", err, "task_uid", task.TaskUID)
			return
		}

		// 指数退避延迟并重新入队
		if s.queue != nil {
			delay := time.Duration(task.RetryIntervalSec) * time.Second * time.Duration(1<<task.RetryCount)
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			if err := s.queue.PushDelayed(ctx, task, delay); err != nil {
				slog.Error("scheduler: failed to re-enqueue task for retry", "error", err, "task_uid", task.TaskUID)
			}
		}

		slog.Info("scheduler: task scheduled for retry",
			"task_uid", task.TaskUID,
			"retry_count", task.RetryCount,
			"max_retries", task.MaxRetries,
		)
		return
	}

	// Terminal failure
	now := time.Now()
	task.FinishedAt = &now

	if err := task.TransitionTo(model.TaskStatusFailed); err != nil {
		slog.Error("scheduler: failed to transition task to failed", "error", err, "task_uid", task.TaskUID)
		return
	}

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task on failure", "error", err, "task_uid", task.TaskUID)
	}

	// 更新每日失败统计
	if s.rdb != nil {
		s.rdb.HIncrBy(ctx, "stats:daily:"+time.Now().Format("2006-01-02"), "failed", 1)
	}
}

// GetTotalResources returns the total available CPU cores and memory in MB.
func (s *Scheduler) GetTotalResources() (cpu, memMB int) {
	return s.totalCPU, s.totalMemMB
}
