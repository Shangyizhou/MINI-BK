package nodemanager

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/pkg/proto"
)

// nodeStore defines the persistence interface needed by NodeManager.
// The concrete store.NodeStore implements this.
type nodeStore interface {
	Create(ctx context.Context, node *model.Node) error
	Update(ctx context.Context, node *model.Node) error
	GetByNodeID(ctx context.Context, nodeID string) (*model.Node, error)
	List(ctx context.Context, status string) ([]*model.Node, error)
	UpdateHeartbeat(ctx context.Context, nodeID string, cpuPct float64, memUsedMB, diskUsedMB int, loadAvg float64, runningTasks int) error
}

// NodeManager manages agent node lifecycle
type NodeManager struct {
	store        nodeStore
	mu           sync.RWMutex
	onlineNodes  map[string]*model.Node // nodeID -> Node (in-memory cache)
	offlineAfter time.Duration          // default 30s
}

// NewNodeManager creates a new NodeManager
func NewNodeManager(store nodeStore) *NodeManager {
	return &NodeManager{
		store:        store,
		onlineNodes:  make(map[string]*model.Node),
		offlineAfter: 30 * time.Second,
	}
}

// Register handles agent registration
func (m *NodeManager) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	node := model.NewNode(req.Hostname, req.Ip, req.Version,
		int(req.TotalCpu), int(req.TotalMemoryMb), int(req.TotalDiskMb),
		req.Labels)

	if err := m.store.Create(ctx, node); err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.onlineNodes[node.NodeID] = node
	m.mu.Unlock()

	slog.Info("节点注册成功", "node_id", node.NodeID, "hostname", node.Hostname)
	return &proto.RegisterResponse{NodeId: node.NodeID, Accepted: true}, nil
}

// Heartbeat updates node resource info
func (m *NodeManager) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) error {
	m.mu.Lock()
	if node, ok := m.onlineNodes[req.NodeId]; ok {
		if req.Resources != nil {
			node.CPUUsagePct = req.Resources.CpuUsagePercent
			node.MemoryUsedMB = int(req.Resources.MemoryUsedMb)
			node.DiskUsedMB = int(req.Resources.DiskUsedMb)
			node.LoadAvg1m = req.Resources.LoadAvg_1M
			node.RunningTasks = int(req.Resources.RunningTasks)
		}
		node.Version = req.Version
		now := time.Now()
		node.LastHeartbeatAt = &now
		node.Status = model.NodeStatusOnline
		m.mu.Unlock()

		if req.Resources != nil {
			return m.store.UpdateHeartbeat(ctx, req.NodeId,
				req.Resources.CpuUsagePercent,
				int(req.Resources.MemoryUsedMb),
				int(req.Resources.DiskUsedMb),
				req.Resources.LoadAvg_1M,
				int(req.Resources.RunningTasks))
		}
		return m.store.UpdateHeartbeat(ctx, req.NodeId, 0, 0, 0, 0, 0)
	}
	m.mu.Unlock()

	// Node not in cache, try to recover from DB
	node, err := m.store.GetByNodeID(ctx, req.NodeId)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.onlineNodes[req.NodeId] = node
	m.mu.Unlock()

	if req.Resources != nil {
		return m.store.UpdateHeartbeat(ctx, req.NodeId,
			req.Resources.CpuUsagePercent,
			int(req.Resources.MemoryUsedMb),
			int(req.Resources.DiskUsedMb),
			req.Resources.LoadAvg_1M,
			int(req.Resources.RunningTasks))
	}
	return m.store.UpdateHeartbeat(ctx, req.NodeId, 0, 0, 0, 0, 0)
}

// StartOfflineChecker periodically marks stale nodes as offline
func (m *NodeManager) StartOfflineChecker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkOfflineNodes(ctx)
		}
	}
}

func (m *NodeManager) checkOfflineNodes(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, node := range m.onlineNodes {
		if node.LastHeartbeatAt != nil && now.Sub(*node.LastHeartbeatAt) > m.offlineAfter {
			node.Status = model.NodeStatusOffline
			if err := m.store.Update(ctx, node); err != nil {
				slog.Error("更新离线节点状态失败", "error", err, "node_id", id)
			}
			delete(m.onlineNodes, id)
			slog.Warn("节点离线", "node_id", id, "hostname", node.Hostname)
		}
	}
}

// splitLabel splits a label string by "=" returning up to 2 parts
func splitLabel(label string) []string {
	return strings.SplitN(label, "=", 2)
}

// FindByLabels returns online nodes matching ALL labels in selector
func (m *NodeManager) FindByLabels(selector map[string]string) []*model.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*model.Node
	for _, node := range m.onlineNodes {
		if !node.IsSchedulable() {
			continue
		}
		match := true
		for k, v := range selector {
			// Try exact match "key=value" or just "value" or just "key"
			if node.HasLabel(k+"="+v) || node.HasLabel(v) || node.HasLabel(k) {
				continue
			}
			// Check each label by splitting
			found := false
			for _, label := range node.Labels {
				parts := splitLabel(label)
				if len(parts) == 2 && parts[0] == k && parts[1] == v {
					found = true
					break
				}
				if label == v {
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

// GetOnlineNodes returns all online nodes
func (m *NodeManager) GetOnlineNodes() []*model.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*model.Node
	for _, node := range m.onlineNodes {
		if node.IsSchedulable() {
			result = append(result, node)
		}
	}
	return result
}

// ListNodes returns all nodes, optionally filtered by status.
func (m *NodeManager) ListNodes(ctx context.Context, status string) ([]*model.Node, error) {
	return m.store.List(ctx, status)
}

// GetNode returns a single node by its node_id.
func (m *NodeManager) GetNode(ctx context.Context, nodeID string) (*model.Node, error) {
	return m.store.GetByNodeID(ctx, nodeID)
}

// DrainNode marks node as drain (no new tasks, existing tasks continue)
func (m *NodeManager) DrainNode(ctx context.Context, nodeID string) error {
	m.mu.Lock()
	if node, ok := m.onlineNodes[nodeID]; ok {
		node.Status = model.NodeStatusDrain
		m.mu.Unlock()
		return m.store.Update(ctx, node)
	}
	m.mu.Unlock()

	node, err := m.store.GetByNodeID(ctx, nodeID)
	if err != nil {
		return err
	}
	node.Status = model.NodeStatusDrain
	return m.store.Update(ctx, node)
}

// UncordonNode restores node to online
func (m *NodeManager) UncordonNode(ctx context.Context, nodeID string) error {
	m.mu.Lock()
	if node, ok := m.onlineNodes[nodeID]; ok {
		node.Status = model.NodeStatusOnline
		m.mu.Unlock()
		return m.store.Update(ctx, node)
	}
	m.mu.Unlock()

	node, err := m.store.GetByNodeID(ctx, nodeID)
	if err != nil {
		return err
	}
	node.Status = model.NodeStatusOnline
	return m.store.Update(ctx, node)
}
