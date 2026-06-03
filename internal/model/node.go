package model

import (
	"time"

	"github.com/google/uuid"
)

// NodeStatus represents the current status of a node.
type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusDrain   NodeStatus = "drain"
	NodeStatusCordon  NodeStatus = "cordon"
)

// Node represents a registered compute node that can execute tasks.
type Node struct {
	ID              int64      `json:"id"`
	NodeID          string     `json:"node_id"`
	Hostname        string     `json:"hostname"`
	IP              string     `json:"ip"`
	Version         string     `json:"version"`
	Status          NodeStatus `json:"status"`
	TotalCPU        int        `json:"total_cpu"`
	TotalMemoryMB   int        `json:"total_memory_mb"`
	TotalDiskMB     int        `json:"total_disk_mb"`
	CPUUsagePct     float64    `json:"cpu_usage_percent"`
	MemoryUsedMB    int        `json:"memory_used_mb"`
	DiskUsedMB      int        `json:"disk_used_mb"`
	LoadAvg1m       float64    `json:"load_avg_1m"`
	RunningTasks    int        `json:"running_tasks"`
	Labels          []string   `json:"labels"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
	RegisteredAt    time.Time  `json:"registered_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// NewNode creates a new Node with an auto-generated UUID and online status.
func NewNode(hostname, ip, version string, totalCPU, totalMemMB, totalDiskMB int, labels []string) *Node {
	now := time.Now()
	return &Node{
		NodeID:        uuid.New().String(),
		Hostname:      hostname,
		IP:            ip,
		Version:       version,
		Status:        NodeStatusOnline,
		TotalCPU:      totalCPU,
		TotalMemoryMB: totalMemMB,
		TotalDiskMB:   totalDiskMB,
		Labels:        labels,
		RegisteredAt:  now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// IsSchedulable returns true if the node can accept tasks.
func (n *Node) IsSchedulable() bool {
	return n.Status == NodeStatusOnline
}

// HasLabel returns true if the node has the specified label.
func (n *Node) HasLabel(label string) bool {
	for _, l := range n.Labels {
		if l == label {
			return true
		}
	}
	return false
}
