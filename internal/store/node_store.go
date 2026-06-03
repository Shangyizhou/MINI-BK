package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"

	"github.com/shangyizhou/mini-bk/internal/model"
)

// NodeStore provides CRUD operations for the nodes table.
type NodeStore struct {
	pg *Postgres
}

// NewNodeStore creates a NodeStore instance.
func NewNodeStore(pg *Postgres) *NodeStore {
	return &NodeStore{pg: pg}
}

// Create inserts a new node record and sets its auto-generated ID.
func (s *NodeStore) Create(ctx context.Context, node *model.Node) error {
	err := s.pg.DB.QueryRowContext(ctx,
		`INSERT INTO nodes (node_id, hostname, ip, version, status,
		 total_cpu, total_memory_mb, total_disk_mb, cpu_usage_percent,
		 memory_used_mb, disk_used_mb, load_avg_1m, running_tasks, labels,
		 registered_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING id`,
		node.NodeID, node.Hostname, node.IP, node.Version, node.Status,
		node.TotalCPU, node.TotalMemoryMB, node.TotalDiskMB, node.CPUUsagePct,
		node.MemoryUsedMB, node.DiskUsedMB, node.LoadAvg1m, node.RunningTasks,
		pq.Array(node.Labels),
		node.RegisteredAt, node.CreatedAt, node.UpdatedAt,
	).Scan(&node.ID)
	return err
}

// Update updates all fields of a node, matched by node_id.
func (s *NodeStore) Update(ctx context.Context, node *model.Node) error {
	node.UpdatedAt = time.Now()

	_, err := s.pg.DB.ExecContext(ctx,
		`UPDATE nodes SET hostname=$1, ip=$2, version=$3, status=$4,
		 total_cpu=$5, total_memory_mb=$6, total_disk_mb=$7,
		 cpu_usage_percent=$8, memory_used_mb=$9, disk_used_mb=$10,
		 load_avg_1m=$11, running_tasks=$12, labels=$13,
		 last_heartbeat_at=$14, updated_at=$15
		 WHERE node_id=$16`,
		node.Hostname, node.IP, node.Version, node.Status,
		node.TotalCPU, node.TotalMemoryMB, node.TotalDiskMB,
		node.CPUUsagePct, node.MemoryUsedMB, node.DiskUsedMB,
		node.LoadAvg1m, node.RunningTasks, pq.Array(node.Labels),
		node.LastHeartbeatAt, node.UpdatedAt,
		node.NodeID,
	)
	return err
}

// GetByID retrieves a node by its database ID.
func (s *NodeStore) GetByID(ctx context.Context, id int64) (*model.Node, error) {
	row := s.pg.DB.QueryRowContext(ctx,
		`SELECT id, node_id, hostname, ip, version, status,
		 total_cpu, total_memory_mb, total_disk_mb, cpu_usage_percent,
		 memory_used_mb, disk_used_mb, load_avg_1m, running_tasks, labels,
		 last_heartbeat_at, registered_at, created_at, updated_at
		 FROM nodes WHERE id=$1`, id)
	return scanNode(row)
}

// GetByNodeID retrieves a node by its UUID node_id.
func (s *NodeStore) GetByNodeID(ctx context.Context, nodeID string) (*model.Node, error) {
	row := s.pg.DB.QueryRowContext(ctx,
		`SELECT id, node_id, hostname, ip, version, status,
		 total_cpu, total_memory_mb, total_disk_mb, cpu_usage_percent,
		 memory_used_mb, disk_used_mb, load_avg_1m, running_tasks, labels,
		 last_heartbeat_at, registered_at, created_at, updated_at
		 FROM nodes WHERE node_id=$1`, nodeID)
	return scanNode(row)
}

// List returns all nodes, optionally filtered by status.
// Returns the list and total count.
func (s *NodeStore) List(ctx context.Context, status string) ([]*model.Node, error) {
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = s.pg.DB.QueryContext(ctx,
			`SELECT id, node_id, hostname, ip, version, status,
			 total_cpu, total_memory_mb, total_disk_mb, cpu_usage_percent,
			 memory_used_mb, disk_used_mb, load_avg_1m, running_tasks, labels,
			 last_heartbeat_at, registered_at, created_at, updated_at
			 FROM nodes ORDER BY registered_at DESC`)
	} else {
		rows, err = s.pg.DB.QueryContext(ctx,
			`SELECT id, node_id, hostname, ip, version, status,
			 total_cpu, total_memory_mb, total_disk_mb, cpu_usage_percent,
			 memory_used_mb, disk_used_mb, load_avg_1m, running_tasks, labels,
			 last_heartbeat_at, registered_at, created_at, updated_at
			 FROM nodes WHERE status=$1 ORDER BY registered_at DESC`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		node, err := scanNodeFromRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

// GetOnlineNodes returns all nodes with status "online".
func (s *NodeStore) GetOnlineNodes(ctx context.Context) ([]*model.Node, error) {
	return s.List(ctx, string(model.NodeStatusOnline))
}

// UpdateHeartbeat updates a node's resource usage and last_heartbeat_at timestamp.
func (s *NodeStore) UpdateHeartbeat(ctx context.Context, nodeID string, cpuPct float64, memUsedMB, diskUsedMB int, loadAvg float64, runningTasks int) error {
	now := time.Now()
	_, err := s.pg.DB.ExecContext(ctx,
		`UPDATE nodes SET cpu_usage_percent=$1, memory_used_mb=$2, disk_used_mb=$3,
		 load_avg_1m=$4, running_tasks=$5, last_heartbeat_at=$6, updated_at=$7
		 WHERE node_id=$8`,
		cpuPct, memUsedMB, diskUsedMB, loadAvg, runningTasks, now, now, nodeID)
	return err
}

// scannableNode interface abstracts sql.Row and sql.Rows for scanning.
type scannableNode interface {
	Scan(dest ...any) error
}

// scanNode reads a single row and assembles a Node.
func scanNode(row scannableNode) (*model.Node, error) {
	node := &model.Node{}
	var labels []string
	var lastHeartbeatAt sql.NullTime

	err := row.Scan(
		&node.ID, &node.NodeID, &node.Hostname, &node.IP, &node.Version,
		&node.Status,
		&node.TotalCPU, &node.TotalMemoryMB, &node.TotalDiskMB,
		&node.CPUUsagePct, &node.MemoryUsedMB, &node.DiskUsedMB,
		&node.LoadAvg1m, &node.RunningTasks,
		pq.Array(&labels),
		&lastHeartbeatAt,
		&node.RegisteredAt, &node.CreatedAt, &node.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if labels == nil {
		node.Labels = []string{}
	} else {
		node.Labels = labels
	}
	if lastHeartbeatAt.Valid {
		node.LastHeartbeatAt = &lastHeartbeatAt.Time
	}

	return node, nil
}

// scanNodeFromRows reads a Node from sql.Rows.
func scanNodeFromRows(rows *sql.Rows) (*model.Node, error) {
	return scanNode(rows)
}
