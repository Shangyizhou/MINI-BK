package discovery

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// NodeEventType represents the type of node event.
type NodeEventType int

const (
	// NodeAdded indicates a new node was added or an existing one was updated.
	NodeAdded NodeEventType = iota
	// NodeUpdated indicates a node's information was updated.
	NodeUpdated
	// NodeRemoved indicates a node was removed (lease expired).
	NodeRemoved
)

// NodeEvent represents a change in the node registry.
type NodeEvent struct {
	Type   NodeEventType
	NodeID string
	Key    string
	Value  []byte
}

// ServiceDiscovery watches etcd for node registration changes and maintains
// an in-memory cache of online nodes.
type ServiceDiscovery struct {
	client    *clientv3.Client
	mu        sync.RWMutex
	nodes     map[string][]byte   // nodeID -> node JSON
	listeners []chan NodeEvent
}

// NewServiceDiscovery creates a new ServiceDiscovery instance.
func NewServiceDiscovery(client *clientv3.Client) *ServiceDiscovery {
	return &ServiceDiscovery{
		client: client,
		nodes:  make(map[string][]byte),
	}
}

// StartWatching begins watching /nodes/ prefix and updating in-memory cache.
// It performs an initial load of all existing nodes, then watches for changes.
func (sd *ServiceDiscovery) StartWatching(ctx context.Context) {
	go func() {
	// 1. Initial load: Get all keys under /nodes/
	resp, err := sd.client.Get(ctx, "/nodes/", clientv3.WithPrefix())
	if err == nil {
		for _, kv := range resp.Kvs {
			nodeID := extractNodeID(string(kv.Key))
			if nodeID == "" {
				continue
			}
			sd.mu.Lock()
			sd.nodes[nodeID] = kv.Value
			sd.mu.Unlock()
			sd.emit(NodeEvent{Type: NodeAdded, NodeID: nodeID, Key: string(kv.Key), Value: kv.Value})
		}
	} else {
		slog.Warn("服务发现初始加载失败", "error", err)
	}

	// 2. Watch for changes
	var watchRev int64
	if resp != nil && resp.Header != nil {
		watchRev = resp.Header.Revision + 1
	}
	go func() {
		watchCh := sd.client.Watch(ctx, "/nodes/", clientv3.WithPrefix(), clientv3.WithRev(watchRev))
		slog.Info("服务发现 Watch 已启动", "prefix", "/nodes/")
		for resp := range watchCh {
			for _, ev := range resp.Events {
				nodeID := extractNodeID(string(ev.Kv.Key))
				if nodeID == "" {
					continue
				}
				switch ev.Type {
				case clientv3.EventTypePut:
					sd.mu.Lock()
					_, existed := sd.nodes[nodeID]
					sd.nodes[nodeID] = ev.Kv.Value
					sd.mu.Unlock()
					eventType := NodeAdded
					if existed {
						eventType = NodeUpdated
					}
					sd.emit(NodeEvent{Type: eventType, NodeID: nodeID, Key: string(ev.Kv.Key), Value: ev.Kv.Value})
				case clientv3.EventTypeDelete:
					sd.mu.Lock()
					delete(sd.nodes, nodeID)
					sd.mu.Unlock()
					sd.emit(NodeEvent{Type: NodeRemoved, NodeID: nodeID, Key: string(ev.Kv.Key)})
				}
			}
		}
	}()
	}()
}

// GetOnlineNodeIDs returns the node IDs of all currently online nodes.
func (sd *ServiceDiscovery) GetOnlineNodeIDs() []string {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	ids := make([]string, 0, len(sd.nodes))
	for id := range sd.nodes {
		ids = append(ids, id)
	}
	return ids
}

// GetNodeInfo returns the raw JSON data for a given node ID.
// Returns the node data and true if found, nil and false otherwise.
func (sd *ServiceDiscovery) GetNodeInfo(nodeID string) ([]byte, bool) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	val, ok := sd.nodes[nodeID]
	return val, ok
}

// Subscribe returns a channel that receives NodeEvents as nodes are added,
// updated, or removed. The channel has a buffer of 100 events. Slow consumers
// may miss events if the buffer fills up.
func (sd *ServiceDiscovery) Subscribe() <-chan NodeEvent {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	ch := make(chan NodeEvent, 100)
	sd.listeners = append(sd.listeners, ch)
	return ch
}

// emit sends a NodeEvent to all registered listeners. Non-blocking: if a
// listener's buffer is full, the event is dropped for that listener.
func (sd *ServiceDiscovery) emit(evt NodeEvent) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	for _, ch := range sd.listeners {
		select {
		case ch <- evt:
		default:
			// Drop event if listener is slow
		}
	}
	_ = evt
}

// extractNodeID extracts the node ID from an etcd key like "/nodes/<nodeID>/status".
func extractNodeID(key string) string {
	// key format: /nodes/<nodeID>/status or /nodes/<nodeID>
	parts := strings.Split(strings.TrimPrefix(key, "/nodes/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}
