package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/queue"
)

// mockTaskStore implements the scheduler's store interface.
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
	// Simulate real store: if task status changed from created, remove from created list
	if task.Status != model.TaskStatusCreated {
		for i, t := range m.created {
			if t == task {
				m.created = append(m.created[:i], m.created[i+1:]...)
				break
			}
		}
	}
	return nil
}

func (m *mockTaskStore) GetByUID(ctx context.Context, uid string) (*model.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Search all task lists
	for _, t := range m.created {
		if t.TaskUID == uid {
			return t, nil
		}
	}
	for _, t := range m.pending {
		if t.TaskUID == uid {
			return t, nil
		}
	}
	for _, t := range m.running {
		if t.TaskUID == uid {
			return t, nil
		}
	}
	return nil, fmt.Errorf("task %s not found", uid)
}

// mockExecutor simulates task execution.
type mockExecutor struct{}

func (m *mockExecutor) Run(ctx context.Context, task *model.Task) *executor.TaskResult {
	code := 0
	return &executor.TaskResult{
		ExitCode: code,
		Stdout:   "mock output",
	}
}

// mockNodeManager implements NodeManager interface for testing.
type mockNodeManager struct {
	mu     sync.Mutex
	nodes  []*model.Node
}

func (m *mockNodeManager) FindByLabels(selector map[string]string) []*model.Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*model.Node
	for _, node := range m.nodes {
		if !node.IsSchedulable() {
			continue
		}
		match := true
		for k, v := range selector {
			found := false
			for _, label := range node.Labels {
				if label == k+"="+v || label == v || label == k {
					found = true
					break
				}
			}
			if !found {
				match = false
				break
			}
		}
		if match {
			result = append(result, node)
		}
	}
	return result
}

func TestScheduler_ScheduleCreatedTask(t *testing.T) {
	store := &mockTaskStore{}
	exec := &mockExecutor{}
	q := queue.NewInMemQueue(10)
	defer q.Close()
	sched := NewScheduler(store, exec, 500*time.Millisecond, 10, nil, q, nil, 0, nil)

	task := model.NewTask("test-schedule", "echo hello")
	task.CPULimit = 1
	task.MemoryLimit = 128
	if err := q.Push(context.Background(), task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	sched.tick(context.Background())

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.updateCalled == 0 {
		t.Error("预期 Update() 被调用（调度器应分发任务）")
	}
}

func TestScheduler_ResourceInsufficient(t *testing.T) {
	store := &mockTaskStore{}
	exec := &mockExecutor{}
	q := queue.NewInMemQueue(10)
	defer q.Close()
	sched := NewScheduler(store, exec, 500*time.Millisecond, 10, nil, q, nil, 0, nil)

	task := model.NewTask("test-heavy", "echo hello")
	task.CPULimit = 999
	task.MemoryLimit = 999999
	if err := q.Push(context.Background(), task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	sched.tick(context.Background())

	// 资源不足时任务应被延迟重新入队，状态保持不变
	if task.Status != model.TaskStatusCreated {
		t.Errorf("Status = %s, 期望 created（资源不足时状态不应改变）", task.Status)
	}
}

func TestScheduler_StartStop(t *testing.T) {
	store := &mockTaskStore{}
	exec := &mockExecutor{}
	q := queue.NewInMemQueue(10)
	defer q.Close()
	sched := NewScheduler(store, exec, 100*time.Millisecond, 10, nil, q, nil, 0, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go sched.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()

	<-time.After(200 * time.Millisecond)
	// 不 panic 就算通过
}

func TestSelectNode_NoNodeManager(t *testing.T) {
	sched := NewScheduler(nil, nil, 100*time.Millisecond, 10, nil, nil, nil, 0, nil)
	task := model.NewTask("test", "echo hello")

	node, err := sched.SelectNode(task)
	if err != nil {
		t.Errorf("SelectNode() error = %v, want nil", err)
	}
	if node != nil {
		t.Errorf("SelectNode() = %v, want nil (no nodeMgr)", node)
	}
}

func TestSelectNode_LabelMatching(t *testing.T) {
	nodeMgr := &mockNodeManager{}
	sched := NewScheduler(nil, nil, 100*time.Millisecond, 10, nil, nil, nil, 0, nil)
	sched.SetNodeManager(nodeMgr)

	// Add candidate nodes
	nodeMgr.nodes = []*model.Node{
		{
			NodeID:      "node-1",
			Hostname:    "worker-1",
			Status:      model.NodeStatusOnline,
			Labels:      []string{"gpu", "region=us-east"},
			TotalCPU:    8,
			TotalMemoryMB: 16384,
			CPUUsagePct: 30.0,
			MemoryUsedMB: 4096,
		},
		{
			NodeID:      "node-2",
			Hostname:    "worker-2",
			Status:      model.NodeStatusOnline,
			Labels:      []string{"high-mem", "region=us-west"},
			TotalCPU:    16,
			TotalMemoryMB: 65536,
			CPUUsagePct: 10.0,
			MemoryUsedMB: 8192,
		},
	}

	t.Run("match by label", func(t *testing.T) {
		task := model.NewTask("test", "echo hello")
		task.NodeSelector = map[string]string{"gpu": ""}

		node, err := sched.SelectNode(task)
		if err != nil {
			t.Fatalf("SelectNode() error = %v", err)
		}
		if node == nil {
			t.Fatal("SelectNode() = nil, want node-1")
		}
		if node.NodeID != "node-1" {
			t.Errorf("SelectNode() = %s, want node-1", node.NodeID)
		}
	})

	t.Run("no matching labels", func(t *testing.T) {
		task := model.NewTask("test", "echo hello")
		task.NodeSelector = map[string]string{"nonexistent": "value"}

		_, err := sched.SelectNode(task)
		if err == nil {
			t.Error("SelectNode() expected error for no matching labels")
		}
	})
}

func TestSelectNode_ResourceFiltering(t *testing.T) {
	nodeMgr := &mockNodeManager{}
	sched := NewScheduler(nil, nil, 100*time.Millisecond, 10, nil, nil, nil, 0, nil)
	sched.SetNodeManager(nodeMgr)

	nodeMgr.nodes = []*model.Node{
		{
			NodeID:        "node-1",
			Hostname:      "small-worker",
			Status:        model.NodeStatusOnline,
			Labels:        []string{"general"},
			TotalCPU:      4,
			TotalMemoryMB: 8192,
			CPUUsagePct:   50.0, // 2 CPU available
			MemoryUsedMB:  4096, // 4GB available
		},
		{
			NodeID:        "node-2",
			Hostname:      "large-worker",
			Status:        model.NodeStatusOnline,
			Labels:        []string{"general"},
			TotalCPU:      16,
			TotalMemoryMB: 65536,
			CPUUsagePct:   10.0, // ~14 CPU available
			MemoryUsedMB:  8192, // ~56GB available
		},
	}

	t.Run("sufficient resources", func(t *testing.T) {
		task := model.NewTask("test", "echo hello")
		task.NodeSelector = map[string]string{"general": ""}
		task.CPULimit = 4
		task.MemoryLimit = 4096

		node, err := sched.SelectNode(task)
		if err != nil {
			t.Fatalf("SelectNode() error = %v", err)
		}
		if node == nil {
			t.Fatal("SelectNode() returned nil")
		}
		// Should pick least utilized (node-2 has lower CPUUsagePct)
		if node.NodeID != "node-2" {
			t.Errorf("SelectNode() = %s, want node-2 (least utilized)", node.NodeID)
		}
	})

	t.Run("insufficient resources on all nodes", func(t *testing.T) {
		task := model.NewTask("test", "echo hello")
		task.NodeSelector = map[string]string{"general": ""}
		task.CPULimit = 99
		task.MemoryLimit = 999999

		_, err := sched.SelectNode(task)
		if err == nil {
			t.Error("SelectNode() expected error for insufficient resources")
		}
	})
}

func TestSelectNode_LeastAllocated(t *testing.T) {
	nodeMgr := &mockNodeManager{}
	sched := NewScheduler(nil, nil, 100*time.Millisecond, 10, nil, nil, nil, 0, nil)
	sched.SetNodeManager(nodeMgr)

	// All nodes have same labels and sufficient resources, different utilization
	nodeMgr.nodes = []*model.Node{
		{
			NodeID:      "node-busy",
			Status:      model.NodeStatusOnline,
			Labels:      []string{"pool"},
			TotalCPU:    8,
			TotalMemoryMB: 16384,
			CPUUsagePct: 90.0,
			MemoryUsedMB: 15000,
		},
		{
			NodeID:      "node-medium",
			Status:      model.NodeStatusOnline,
			Labels:      []string{"pool"},
			TotalCPU:    8,
			TotalMemoryMB: 16384,
			CPUUsagePct: 50.0,
			MemoryUsedMB: 8000,
		},
		{
			NodeID:      "node-idle",
			Status:      model.NodeStatusOnline,
			Labels:      []string{"pool"},
			TotalCPU:    8,
			TotalMemoryMB: 16384,
			CPUUsagePct: 10.0,
			MemoryUsedMB: 2000,
		},
	}

	task := model.NewTask("test", "echo hello")
	task.NodeSelector = map[string]string{"pool": ""}
	task.CPULimit = 1
	task.MemoryLimit = 512

	node, err := sched.SelectNode(task)
	if err != nil {
		t.Fatalf("SelectNode() error = %v", err)
	}
	if node == nil {
		t.Fatal("SelectNode() returned nil")
	}
	// Should pick the least utilized node (node-idle with 10% CPU)
	if node.NodeID != "node-idle" {
		t.Errorf("SelectNode() = %s, want node-idle (least utilized)", node.NodeID)
	}
}

func TestSelectNode_SchedulableOnly(t *testing.T) {
	nodeMgr := &mockNodeManager{}
	sched := NewScheduler(nil, nil, 100*time.Millisecond, 10, nil, nil, nil, 0, nil)
	sched.SetNodeManager(nodeMgr)

	nodeMgr.nodes = []*model.Node{
		{
			NodeID: "node-online",
			Status: model.NodeStatusOnline,
			Labels: []string{"pool"},
		},
		{
			NodeID: "node-drain",
			Status: model.NodeStatusDrain,
			Labels: []string{"pool"},
		},
		{
			NodeID: "node-offline",
			Status: model.NodeStatusOffline,
			Labels: []string{"pool"},
		},
	}

	task := model.NewTask("test", "echo hello")
	task.NodeSelector = map[string]string{"pool": ""}

	node, err := sched.SelectNode(task)
	if err != nil {
		t.Fatalf("SelectNode() error = %v", err)
	}
	if node == nil {
		t.Fatal("SelectNode() returned nil")
	}
	if node.NodeID != "node-online" {
		t.Errorf("SelectNode() = %s, want node-online", node.NodeID)
	}
}

func TestDispatch_NodeMgrFallback(t *testing.T) {
	store := &mockTaskStore{}
	exec := &mockExecutor{}
	q := queue.NewInMemQueue(10)
	defer q.Close()
	sched := NewScheduler(store, exec, 500*time.Millisecond, 10, nil, q, nil, 0, nil)

	task := model.NewTask("test-fallback", "echo hello")
	task.CPULimit = 1
	task.MemoryLimit = 128

	if err := q.Push(context.Background(), task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// With nil nodeMgr, should fall back to local execution
	sched.tick(context.Background())

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.updateCalled == 0 {
		t.Error("expected Update() to be called (local execution fallback)")
	}
}
