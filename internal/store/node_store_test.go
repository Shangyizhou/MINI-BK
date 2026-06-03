package store

import (
	"context"
	"testing"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
)

func setupNodeStore(t *testing.T) (*NodeStore, func()) {
	t.Helper()
	dsn := "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable"
	pg, err := NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Skipf("跳过：无法连接 PostgreSQL: %v", err)
	}
	// Run migration to ensure table exists
	_, err = pg.DB.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS nodes (
			id BIGSERIAL PRIMARY KEY,
			node_id VARCHAR(36) NOT NULL UNIQUE,
			hostname VARCHAR(255) NOT NULL,
			ip VARCHAR(45) NOT NULL,
			version VARCHAR(20) DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'offline',
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
		)`)
	if err != nil {
		t.Skipf("跳过：无法创建表: %v", err)
	}
	// Clean test data
	pg.DB.ExecContext(context.Background(), "DELETE FROM nodes")
	store := NewNodeStore(pg)
	return store, func() {
		pg.DB.ExecContext(context.Background(), "DELETE FROM nodes")
		pg.Close()
	}
}

func TestNodeStore_Create(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	node := model.NewNode("worker-01", "10.0.0.1", "0.3.0", 8, 16384, 102400, []string{"gpu", "high-mem"})
	if err := store.Create(context.Background(), node); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if node.ID == 0 {
		t.Error("Create() 应该设置 ID")
	}

	// Verify by reading back
	got, err := store.GetByNodeID(context.Background(), node.NodeID)
	if err != nil {
		t.Fatalf("GetByNodeID() error = %v", err)
	}
	if got.Hostname != "worker-01" {
		t.Errorf("Hostname = %s, 期望 worker-01", got.Hostname)
	}
	if got.Status != model.NodeStatusOnline {
		t.Errorf("Status = %s, 期望 online", got.Status)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "gpu" {
		t.Errorf("Labels = %v, 期望 [gpu high-mem]", got.Labels)
	}
}

func TestNodeStore_GetByID(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	node := model.NewNode("worker-02", "10.0.0.2", "0.3.0", 4, 8192, 51200, nil)
	if err := store.Create(context.Background(), node); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.NodeID != node.NodeID {
		t.Errorf("NodeID = %s, 期望 %s", got.NodeID, node.NodeID)
	}
}

func TestNodeStore_GetByNodeID_NotFound(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	_, err := store.GetByNodeID(context.Background(), "nonexistent-uuid")
	if err == nil {
		t.Error("GetByNodeID() 期望错误但得到 nil")
	}
}

func TestNodeStore_Update(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	node := model.NewNode("worker-03", "10.0.0.3", "0.3.0", 8, 16384, 102400, []string{"gpu"})
	if err := store.Create(context.Background(), node); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update node fields
	node.Status = model.NodeStatusDrain
	node.CPUUsagePct = 45.5
	node.MemoryUsedMB = 4096
	node.RunningTasks = 3
	if err := store.Update(context.Background(), node); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := store.GetByNodeID(context.Background(), node.NodeID)
	if err != nil {
		t.Fatalf("GetByNodeID() error = %v", err)
	}
	if got.Status != model.NodeStatusDrain {
		t.Errorf("Status = %s, 期望 drain", got.Status)
	}
	if got.CPUUsagePct != 45.5 {
		t.Errorf("CPUUsagePct = %f, 期望 45.5", got.CPUUsagePct)
	}
	if got.MemoryUsedMB != 4096 {
		t.Errorf("MemoryUsedMB = %d, 期望 4096", got.MemoryUsedMB)
	}
}

func TestNodeStore_List(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	// Create multiple nodes
	for i := 0; i < 3; i++ {
		node := model.NewNode("worker-list", "10.0.0.1", "0.3.0", 4, 8192, 51200, nil)
		if err := store.Create(context.Background(), node); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	nodes, err := store.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("len(nodes) = %d, 期望 3", len(nodes))
	}
}

func TestNodeStore_ListByStatus(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	// Create two nodes with different statuses
	node1 := model.NewNode("worker-online", "10.0.0.1", "0.3.0", 4, 8192, 51200, nil)
	if err := store.Create(context.Background(), node1); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	node2 := model.NewNode("worker-drain", "10.0.0.2", "0.3.0", 4, 8192, 51200, nil)
	node2.Status = model.NodeStatusDrain
	if err := store.Create(context.Background(), node2); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Filter by online status
	nodes, err := store.List(context.Background(), string(model.NodeStatusOnline))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("len(nodes) = %d, 期望 1", len(nodes))
	}
	if nodes[0].Status != model.NodeStatusOnline {
		t.Errorf("Status = %s, 期望 online", nodes[0].Status)
	}
}

func TestNodeStore_GetOnlineNodes(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	// Create online and offline nodes
	node1 := model.NewNode("worker-online", "10.0.0.1", "0.3.0", 4, 8192, 51200, nil)
	if err := store.Create(context.Background(), node1); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	node2 := model.NewNode("worker-offline", "10.0.0.2", "0.3.0", 4, 8192, 51200, nil)
	node2.Status = model.NodeStatusOffline
	if err := store.Create(context.Background(), node2); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	nodes, err := store.GetOnlineNodes(context.Background())
	if err != nil {
		t.Fatalf("GetOnlineNodes() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("len(nodes) = %d, 期望 1", len(nodes))
	}
}

func TestNodeStore_UpdateHeartbeat(t *testing.T) {
	store, cleanup := setupNodeStore(t)
	defer cleanup()

	node := model.NewNode("worker-hb", "10.0.0.1", "0.3.0", 8, 16384, 102400, nil)
	if err := store.Create(context.Background(), node); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := store.UpdateHeartbeat(context.Background(), node.NodeID, 23.5, 2048, 51200, 0.5, 2)
	if err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}

	got, err := store.GetByNodeID(context.Background(), node.NodeID)
	if err != nil {
		t.Fatalf("GetByNodeID() error = %v", err)
	}
	if got.CPUUsagePct != 23.5 {
		t.Errorf("CPUUsagePct = %f, 期望 23.5", got.CPUUsagePct)
	}
	if got.MemoryUsedMB != 2048 {
		t.Errorf("MemoryUsedMB = %d, 期望 2048", got.MemoryUsedMB)
	}
	if got.RunningTasks != 2 {
		t.Errorf("RunningTasks = %d, 期望 2", got.RunningTasks)
	}
	if got.LastHeartbeatAt == nil || got.LastHeartbeatAt.Before(time.Now().Add(-time.Minute)) {
		t.Error("LastHeartbeatAt 应为最近时间")
	}
}
