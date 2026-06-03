package store

import (
	"context"
	"testing"

	"github.com/shangyizhou/mini-bk/internal/model"
)

func setupTaskStore(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	dsn := "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable"
	pg, err := NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Skipf("跳过：无法连接 PostgreSQL: %v", err)
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
		t.Error("Create() 应该设置 ID")
	}

	// 验证可读回
	got, err := store.GetByUID(context.Background(), task.TaskUID)
	if err != nil {
		t.Fatalf("GetByUID() error = %v", err)
	}
	if got.Name != "test-create" {
		t.Errorf("Name = %s, 期望 test-create", got.Name)
	}
	if got.Priority != 5 {
		t.Errorf("Priority = %d, 期望 5", got.Priority)
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
		t.Errorf("total = %d, 期望 5", total)
	}
	if len(tasks) != 3 {
		t.Errorf("len(tasks) = %d, 期望 3（分页大小）", len(tasks))
	}
}

func TestTaskStore_ListByStatus(t *testing.T) {
	store, cleanup := setupTaskStore(t)
	defer cleanup()

	task := model.NewTask("test-status", "echo hello")
	task.Status = model.TaskStatusRunning
	store.Create(context.Background(), task)

	_, total, err := store.List(context.Background(), "running", 1, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, 期望 1", total)
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
		t.Errorf("Status = %s, 期望 pending", got.Status)
	}
}

func TestTaskStore_GetPendingTasks(t *testing.T) {
	store, cleanup := setupTaskStore(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		task := model.NewTask("test-pending", "echo hello")
		task.Status = model.TaskStatusPending
		store.Create(context.Background(), task)
	}
	task := model.NewTask("test-created", "echo hello")
	store.Create(context.Background(), task)

	tasks, err := store.GetPendingTasks(context.Background())
	if err != nil {
		t.Fatalf("GetPendingTasks() error = %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("len(tasks) = %d, 期望 3", len(tasks))
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
		t.Errorf("len(tasks) = %d, 期望 1", len(tasks))
	}
	if tasks[0].CPULimit != 2 {
		t.Errorf("CPULimit = %d, 期望 2", tasks[0].CPULimit)
	}
}

func TestTaskStore_NodeSelectorRoundTrip(t *testing.T) {
	store, cleanup := setupTaskStore(t)
	defer cleanup()

	task := model.NewTask("test-node-selector", "echo hello")
	task.NodeSelector = map[string]string{"os": "linux", "region": "us-east-1"}
	task.AssignedNodeID = "node-001"

	if err := store.Create(context.Background(), task); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if task.ID == 0 {
		t.Error("Create() 应该设置 ID")
	}

	// 通过 GetByUID 验证读取
	got, err := store.GetByUID(context.Background(), task.TaskUID)
	if err != nil {
		t.Fatalf("GetByUID() error = %v", err)
	}
	if len(got.NodeSelector) != 2 {
		t.Errorf("NodeSelector 长度 = %d, 期望 2", len(got.NodeSelector))
	}
	if got.NodeSelector["os"] != "linux" {
		t.Errorf("NodeSelector[os] = %s, 期望 linux", got.NodeSelector["os"])
	}
	if got.NodeSelector["region"] != "us-east-1" {
		t.Errorf("NodeSelector[region] = %s, 期望 us-east-1", got.NodeSelector["region"])
	}
	if got.AssignedNodeID != "node-001" {
		t.Errorf("AssignedNodeID = %s, 期望 node-001", got.AssignedNodeID)
	}

	// 验证列表查询也返回 node_selector
	tasks, _, err := store.List(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	found := false
	for _, tsk := range tasks {
		if tsk.TaskUID == task.TaskUID {
			found = true
			if tsk.NodeSelector["os"] != "linux" {
				t.Errorf("List 中 NodeSelector[os] = %s, 期望 linux", tsk.NodeSelector["os"])
			}
			if tsk.AssignedNodeID != "node-001" {
				t.Errorf("List 中 AssignedNodeID = %s, 期望 node-001", tsk.AssignedNodeID)
			}
			break
		}
	}
	if !found {
		t.Error("List 结果中未找到测试任务")
	}

	// 验证 Update 也处理 node_selector
	task.AssignedNodeID = "node-002"
	if err := store.Update(context.Background(), task); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got2, _ := store.GetByUID(context.Background(), task.TaskUID)
	if got2.AssignedNodeID != "node-002" {
		t.Errorf("Update 后 AssignedNodeID = %s, 期望 node-002", got2.AssignedNodeID)
	}
}
