package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/shangyizhou/mini-bk/internal/model"
)

type mockNodeService struct {
	nodes map[string]*model.Node
}

func newMockNodeService() *mockNodeService {
	return &mockNodeService{nodes: make(map[string]*model.Node)}
}

func (m *mockNodeService) ListNodes(ctx context.Context, status string) ([]*model.Node, error) {
	var result []*model.Node
	for _, node := range m.nodes {
		if status == "" || string(node.Status) == status {
			result = append(result, node)
		}
	}
	return result, nil
}

func (m *mockNodeService) GetNode(ctx context.Context, nodeID string) (*model.Node, error) {
	node, ok := m.nodes[nodeID]
	if !ok {
		return nil, model.ErrTaskNotFound // reuse task not found error as a generic not-found
	}
	return node, nil
}

func (m *mockNodeService) DrainNode(ctx context.Context, nodeID string) error {
	node, ok := m.nodes[nodeID]
	if !ok {
		return model.ErrTaskNotFound
	}
	node.Status = model.NodeStatusDrain
	return nil
}

func (m *mockNodeService) UncordonNode(ctx context.Context, nodeID string) error {
	node, ok := m.nodes[nodeID]
	if !ok {
		return model.ErrTaskNotFound
	}
	node.Status = model.NodeStatusOnline
	return nil
}

func setupNodeTestRouter(nodeSvc *mockNodeService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r, nil, nil, nil, nil, nodeSvc)
	return r
}

func TestListNodesHandler(t *testing.T) {
	nodeSvc := newMockNodeService()
	nodeSvc.nodes["node-1"] = &model.Node{
		NodeID:   "node-1",
		Hostname: "worker-1",
		Status:   model.NodeStatusOnline,
	}
	nodeSvc.nodes["node-2"] = &model.Node{
		NodeID:   "node-2",
		Hostname: "worker-2",
		Status:   model.NodeStatusOnline,
	}
	nodeSvc.nodes["node-3"] = &model.Node{
		NodeID:   "node-3",
		Hostname: "worker-3",
		Status:   model.NodeStatusDrain,
	}
	router := setupNodeTestRouter(nodeSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["nodes"] == nil {
		t.Error("响应中应包含 nodes")
	}
	nodes, ok := resp["nodes"].([]interface{})
	if !ok || len(nodes) != 3 {
		t.Errorf("nodes 数量 = %d, 期望 3", len(nodes))
	}
}

func TestListNodesHandler_FilterByStatus(t *testing.T) {
	nodeSvc := newMockNodeService()
	nodeSvc.nodes["node-1"] = &model.Node{
		NodeID:   "node-1",
		Hostname: "worker-1",
		Status:   model.NodeStatusOnline,
	}
	nodeSvc.nodes["node-2"] = &model.Node{
		NodeID:   "node-2",
		Hostname: "worker-2",
		Status:   model.NodeStatusDrain,
	}
	router := setupNodeTestRouter(nodeSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?status=online", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	nodes, ok := resp["nodes"].([]interface{})
	if !ok || len(nodes) != 1 {
		t.Errorf("nodes 数量 = %d, 期望 1（只过滤 online）", len(nodes))
	}
}

func TestGetNodeHandler(t *testing.T) {
	nodeSvc := newMockNodeService()
	nodeSvc.nodes["node-1"] = &model.Node{
		NodeID:   "node-1",
		Hostname: "worker-1",
		Status:   model.NodeStatusOnline,
	}
	router := setupNodeTestRouter(nodeSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}

	var node model.Node
	json.Unmarshal(w.Body.Bytes(), &node)
	if node.NodeID != "node-1" {
		t.Errorf("NodeID = %s, 期望 node-1", node.NodeID)
	}
}

func TestGetNodeHandler_NotFound(t *testing.T) {
	nodeSvc := newMockNodeService()
	router := setupNodeTestRouter(nodeSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusNotFound)
	}
}

func TestDrainNodeHandler(t *testing.T) {
	nodeSvc := newMockNodeService()
	nodeSvc.nodes["node-1"] = &model.Node{
		NodeID:   "node-1",
		Hostname: "worker-1",
		Status:   model.NodeStatusOnline,
	}
	router := setupNodeTestRouter(nodeSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/drain", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}

	// Verify node status changed
	node, _ := nodeSvc.GetNode(context.Background(), "node-1")
	if node.Status != model.NodeStatusDrain {
		t.Errorf("Status = %s, 期望 drain", node.Status)
	}
}

func TestUncordonNodeHandler(t *testing.T) {
	nodeSvc := newMockNodeService()
	nodeSvc.nodes["node-1"] = &model.Node{
		NodeID:   "node-1",
		Hostname: "worker-1",
		Status:   model.NodeStatusDrain,
	}
	router := setupNodeTestRouter(nodeSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/uncordon", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}

	// Verify node status changed back
	node, _ := nodeSvc.GetNode(context.Background(), "node-1")
	if node.Status != model.NodeStatusOnline {
		t.Errorf("Status = %s, 期望 online", node.Status)
	}
}
