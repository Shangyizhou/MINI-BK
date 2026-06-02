package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/queue"
)

// mockTaskStore 实现调度器所需的 store 接口
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

// mockExecutor 模拟执行器
type mockExecutor struct{}

func (m *mockExecutor) Run(ctx context.Context, task *model.Task) *executor.TaskResult {
	code := 0
	return &executor.TaskResult{
		ExitCode: code,
		Stdout:   "mock output",
	}
}

func TestScheduler_ScheduleCreatedTask(t *testing.T) {
	store := &mockTaskStore{}
	exec := &mockExecutor{}
	q := queue.NewInMemQueue(10)
	defer q.Close()
	sched := NewScheduler(store, exec, 500*time.Millisecond, 10, nil, q)

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
	sched := NewScheduler(store, exec, 500*time.Millisecond, 10, nil, q)

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
	sched := NewScheduler(store, exec, 100*time.Millisecond, 10, nil, q)

	ctx, cancel := context.WithCancel(context.Background())
	go sched.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()

	<-time.After(200 * time.Millisecond)
	// 不 panic 就算通过
}
