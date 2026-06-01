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
		t.Error("TaskUID 不应为空")
	}
	if task.Name != "test-task" {
		t.Errorf("Name = %s, 期望 test-task", task.Name)
	}
	if task.Status != model.TaskStatusCreated {
		t.Errorf("Status = %s, 期望 created", task.Status)
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
		t.Errorf("TaskUID 不匹配")
	}
}

func TestTaskService_GetTaskNotFound(t *testing.T) {
	svc := NewTaskService(newMockStore())
	_, err := svc.GetTask(context.Background(), "nonexistent")
	if err == nil {
		t.Error("GetTask() 应对不存在的任务返回错误")
	}
}

func TestTaskService_CancelTask(t *testing.T) {
	store := newMockStore()
	svc := NewTaskService(store)

	task, _ := svc.CreateTask(context.Background(), CreateTaskRequest{
		Name:    "test-cancel",
		Command: "sleep 100",
	})
	task.TransitionTo(model.TaskStatusPending)
	task.TransitionTo(model.TaskStatusRunning)
	store.Update(context.Background(), task)

	err := svc.CancelTask(context.Background(), task.TaskUID)
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}

	got, _ := svc.GetTask(context.Background(), task.TaskUID)
	if got.Status != model.TaskStatusCanceled {
		t.Errorf("Status = %s, 期望 canceled", got.Status)
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
		t.Error("重跑任务应有不同的 TaskUID")
	}
	if rerun.Name != "test-rerun" {
		t.Errorf("Name = %s, 期望 test-rerun", rerun.Name)
	}
	if rerun.Command != "echo hello" {
		t.Errorf("Command = %s, 期望 echo hello", rerun.Command)
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
		t.Errorf("Total = %d, 期望 5", result.Total)
	}
	if len(result.Tasks) != 3 {
		t.Errorf("len(Tasks) = %d, 期望 3", len(result.Tasks))
	}
}
