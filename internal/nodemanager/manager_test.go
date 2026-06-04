package nodemanager

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/pkg/proto"
)

// mockStore implements nodeStore interface for testing
type mockStore struct {
	mu           sync.Mutex
	nodes        map[string]*model.Node
	createCalled int
	updateCalled int
	hbCalled     int
}

func newMockStore() *mockStore {
	return &mockStore{
		nodes: make(map[string]*model.Node),
	}
}

func (m *mockStore) Create(ctx context.Context, node *model.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	node.ID = int64(len(m.nodes) + 1)
	m.nodes[node.NodeID] = node
	m.createCalled++
	return nil
}

func (m *mockStore) Update(ctx context.Context, node *model.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes[node.NodeID] = node
	m.updateCalled++
	return nil
}

func (m *mockStore) GetByNodeID(ctx context.Context, nodeID string) (*model.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if node, ok := m.nodes[nodeID]; ok {
		return node, nil
	}
	return nil, model.ErrTaskNotFound
}

func (m *mockStore) List(ctx context.Context, status string) ([]*model.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*model.Node
	for _, node := range m.nodes {
		if status == "" || string(node.Status) == status {
			result = append(result, node)
		}
	}
	return result, nil
}

func (m *mockStore) UpdateHeartbeat(ctx context.Context, nodeID string, cpuPct float64, memUsedMB, diskUsedMB int, loadAvg float64, runningTasks int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hbCalled++
	if node, ok := m.nodes[nodeID]; ok {
		node.CPUUsagePct = cpuPct
		node.MemoryUsedMB = memUsedMB
		node.DiskUsedMB = diskUsedMB
		node.LoadAvg1m = loadAvg
		node.RunningTasks = runningTasks
	}
	return nil
}

func TestNodeManager_Register(t *testing.T) {
	ms := newMockStore()
	nm := NewNodeManager(ms, nil)

	resp, err := nm.Register(context.Background(), &proto.RegisterRequest{
		Hostname:      "test-agent",
		Ip:            "10.0.0.1",
		Version:       "0.3.0",
		TotalCpu:      8,
		TotalMemoryMb: 16384,
		TotalDiskMb:   102400,
		Labels:        []string{"gpu", "region=us-east"},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !resp.Accepted {
		t.Error("Register() should be accepted")
	}
	if resp.NodeId == "" {
		t.Error("Register() should return a node ID")
	}

	// Verify in-memory cache
	nm.mu.RLock()
	node, ok := nm.onlineNodes[resp.NodeId]
	nm.mu.RUnlock()
	if !ok {
		t.Error("node should be in online cache")
	}
	if node.Hostname != "test-agent" {
		t.Errorf("Hostname = %s, want test-agent", node.Hostname)
	}
	if node.Status != model.NodeStatusOnline {
		t.Errorf("Status = %s, want online", node.Status)
	}

	// Verify store was called
	if ms.createCalled != 1 {
		t.Errorf("store.Create called %d times, want 1", ms.createCalled)
	}
}

func TestNodeManager_Register_Duplicate(t *testing.T) {
	ms := newMockStore()
	nm := NewNodeManager(ms, nil)

	// Register once
	resp1, _ := nm.Register(context.Background(), &proto.RegisterRequest{
		Hostname: "agent-1",
		Ip:       "10.0.0.1",
		Version:  "0.3.0",
	})
	_ = resp1

	// Register again (different hostname, will create a new node since each gets UUID)
	resp2, err := nm.Register(context.Background(), &proto.RegisterRequest{
		Hostname: "agent-2",
		Ip:       "10.0.0.2",
		Version:  "0.3.0",
	})
	if err != nil {
		t.Fatalf("Second Register() error = %v", err)
	}
	if !resp2.Accepted {
		t.Error("Second Register() should be accepted")
	}

	if ms.createCalled != 2 {
		t.Errorf("store.Create called %d times, want 2", ms.createCalled)
	}
}

func TestNodeManager_Heartbeat(t *testing.T) {
	ms := newMockStore()
	nm := NewNodeManager(ms, nil)

	// Register first
	resp, _ := nm.Register(context.Background(), &proto.RegisterRequest{
		Hostname: "test-agent",
		Ip:       "10.0.0.1",
		Version:  "0.3.0",
	})

	// Heartbeat
	err := nm.Heartbeat(context.Background(), &proto.HeartbeatRequest{
		NodeId:  resp.NodeId,
		Version: "0.4.0",
		Resources: &proto.NodeResource{
			CpuUsagePercent: 55.5,
			MemoryUsedMb:    4096,
			DiskUsedMb:      20480,
			LoadAvg_1M:      2.5,
			RunningTasks:    5,
		},
	})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}

	// Verify cache updated
	nm.mu.RLock()
	node, ok := nm.onlineNodes[resp.NodeId]
	nm.mu.RUnlock()
	if !ok {
		t.Error("node should be in online cache")
	}
	if node.CPUUsagePct != 55.5 {
		t.Errorf("CPUUsagePct = %f, want 55.5", node.CPUUsagePct)
	}
	if node.MemoryUsedMB != 4096 {
		t.Errorf("MemoryUsedMB = %d, want 4096", node.MemoryUsedMB)
	}
	if node.RunningTasks != 5 {
		t.Errorf("RunningTasks = %d, want 5", node.RunningTasks)
	}
	if node.Version != "0.4.0" {
		t.Errorf("Version = %s, want 0.4.0", node.Version)
	}
	if node.LastHeartbeatAt == nil {
		t.Error("LastHeartbeatAt should be set")
	}

	// Verify store heartbeat was called
	if ms.hbCalled != 1 {
		t.Errorf("store.UpdateHeartbeat called %d times, want 1", ms.hbCalled)
	}
}

func TestNodeManager_Heartbeat_RecoverFromDB(t *testing.T) {
	ms := newMockStore()
	nm := NewNodeManager(ms, nil)

	// Create node directly in mock store (simulating DB recovery path)
	node := model.NewNode("recovered-agent", "10.0.0.2", "0.3.0", 4, 8192, 51200, nil)
	ms.nodes[node.NodeID] = node

	// Heartbeat with node not in cache should recover from store
	err := nm.Heartbeat(context.Background(), &proto.HeartbeatRequest{
		NodeId:  node.NodeID,
		Version: "0.4.0",
		Resources: &proto.NodeResource{
			CpuUsagePercent: 30.0,
		},
	})
	if err != nil {
		t.Fatalf("Heartbeat() recovery error = %v", err)
	}

	// Node should now be in cache
	nm.mu.RLock()
	cached, ok := nm.onlineNodes[node.NodeID]
	nm.mu.RUnlock()
	if !ok {
		t.Error("node should be recovered into online cache")
	}
	if cached.CPUUsagePct != 30.0 {
		t.Errorf("CPUUsagePct = %f, want 30.0", cached.CPUUsagePct)
	}
}

func TestNodeManager_FindByLabels(t *testing.T) {
	nm := &NodeManager{
		onlineNodes: make(map[string]*model.Node),
	}

	// Add nodes with different labels
	nm.onlineNodes["node-1"] = &model.Node{
		NodeID:   "node-1",
		Hostname: "worker-1",
		Status:   model.NodeStatusOnline,
		Labels:   []string{"gpu", "region=us-east"},
		TotalCPU: 8,
	}
	nm.onlineNodes["node-2"] = &model.Node{
		NodeID:   "node-2",
		Hostname: "worker-2",
		Status:   model.NodeStatusOnline,
		Labels:   []string{"high-mem", "region=us-west"},
		TotalCPU: 16,
	}
	nm.onlineNodes["node-3"] = &model.Node{
		NodeID:   "node-3",
		Hostname: "worker-3",
		Status:   model.NodeStatusDrain,
		Labels:   []string{"gpu", "region=us-east"},
		TotalCPU: 8,
	}

	tests := []struct {
		name     string
		selector map[string]string
		want     int
	}{
		{
			name:     "match gpu label",
			selector: map[string]string{"gpu": ""},
			want:     1, // node-1 is online with "gpu", node-3 is drain
		},
		{
			name:     "match region key-value",
			selector: map[string]string{"region": "us-east"},
			want:     1, // node-1
		},
		{
			name:     "match all",
			selector: map[string]string{"gpu": "", "region": "us-east"},
			want:     1, // node-1 matches both
		},
		{
			name:     "no match",
			selector: map[string]string{"nonexistent": "value"},
			want:     0,
		},
		{
			name:     "empty selector returns all schedulable",
			selector: map[string]string{},
			want:     2, // node-1 and node-2 (online), node-3 is drain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nm.FindByLabels(tt.selector)
			if len(got) != tt.want {
				t.Errorf("FindByLabels() = %d nodes, want %d", len(got), tt.want)
			}
		})
	}
}

func TestNodeManager_GetOnlineNodes(t *testing.T) {
	nm := &NodeManager{
		onlineNodes: make(map[string]*model.Node),
	}

	nm.onlineNodes["node-1"] = &model.Node{
		NodeID: "node-1",
		Status: model.NodeStatusOnline,
	}
	nm.onlineNodes["node-2"] = &model.Node{
		NodeID: "node-2",
		Status: model.NodeStatusDrain,
	}
	nm.onlineNodes["node-3"] = &model.Node{
		NodeID: "node-3",
		Status: model.NodeStatusOffline,
	}

	nodes := nm.GetOnlineNodes()
	if len(nodes) != 1 {
		t.Errorf("GetOnlineNodes() = %d nodes, want 1", len(nodes))
	}
	if nodes[0].NodeID != "node-1" {
		t.Errorf("expected node-1, got %s", nodes[0].NodeID)
	}
}

func TestSplitLabel(t *testing.T) {
	tests := []struct {
		label string
		want  int
	}{
		{"gpu", 1},
		{"region=us-east", 2},
		{"key=value=extra", 2},
	}

	for _, tt := range tests {
		parts := splitLabel(tt.label)
		if len(parts) != tt.want {
			t.Errorf("splitLabel(%q) = %d parts, want %d", tt.label, len(parts), tt.want)
		}
	}
}

func TestNodeManager_CheckOfflineNodes(t *testing.T) {
	ms := newMockStore()
	nm := NewNodeManager(ms, nil)
	nm.offlineAfter = 30 * time.Second

	now := time.Now()
	past := now.Add(-60 * time.Second) // 60s ago, beyond 30s threshold

	nm.mu.Lock()
	nm.onlineNodes["node-1"] = &model.Node{
		NodeID:          "node-1",
		LastHeartbeatAt: &past,
		Status:          model.NodeStatusOnline,
	}
	nm.onlineNodes["node-2"] = &model.Node{
		NodeID:          "node-2",
		LastHeartbeatAt: &now,
		Status:          model.NodeStatusOnline,
	}
	nm.onlineNodes["node-3"] = &model.Node{
		NodeID:          "node-3",
		LastHeartbeatAt: nil,
		Status:          model.NodeStatusOnline,
	}
	nm.mu.Unlock()

	nm.checkOfflineNodes(context.Background())

	nm.mu.RLock()
	_, node1online := nm.onlineNodes["node-1"]
	_, node2online := nm.onlineNodes["node-2"]
	_, node3online := nm.onlineNodes["node-3"]
	nm.mu.RUnlock()

	if node1online {
		t.Error("node-1 should have been removed (stale heartbeat)")
	}
	if !node2online {
		t.Error("node-2 should still be online (recent heartbeat)")
	}
	if !node3online {
		t.Error("node-3 should still be online (no heartbeat yet)")
	}

	if ms.updateCalled != 1 {
		t.Errorf("store.Update called %d times, want 1", ms.updateCalled)
	}
}

func TestNodeManager_DrainUncordon(t *testing.T) {
	ms := newMockStore()
	nm := NewNodeManager(ms, nil)

	// Register first
	resp, _ := nm.Register(context.Background(), &proto.RegisterRequest{
		Hostname: "test-agent",
		Ip:       "10.0.0.1",
		Version:  "0.3.0",
	})

	// Drain
	err := nm.DrainNode(context.Background(), resp.NodeId)
	if err != nil {
		t.Fatalf("DrainNode() error = %v", err)
	}

	nm.mu.RLock()
	if nm.onlineNodes[resp.NodeId].Status != model.NodeStatusDrain {
		t.Errorf("Status = %s, want drain", nm.onlineNodes[resp.NodeId].Status)
	}
	nm.mu.RUnlock()

	// Check store was updated
	if ms.updateCalled < 1 {
		t.Error("store.Update should have been called for drain")
	}

	// Uncordon
	err = nm.UncordonNode(context.Background(), resp.NodeId)
	if err != nil {
		t.Fatalf("UncordonNode() error = %v", err)
	}

	nm.mu.RLock()
	if nm.onlineNodes[resp.NodeId].Status != model.NodeStatusOnline {
		t.Errorf("Status = %s, want online", nm.onlineNodes[resp.NodeId].Status)
	}
	nm.mu.RUnlock()
}
