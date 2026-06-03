package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
)

// TaskStore 提供对 tasks 表的 CRUD 操作。
type TaskStore struct {
	pg *Postgres
}

// NewTaskStore 创建 TaskStore 实例。
func NewTaskStore(pg *Postgres) *TaskStore {
	return &TaskStore{pg: pg}
}

// Create 创建一个新任务，并设置其自增 ID。
func (s *TaskStore) Create(ctx context.Context, task *model.Task) error {
	envJSON, err := json.Marshal(task.Env)
	if err != nil {
		return err
	}

	nodeSelectorJSON, err := json.Marshal(task.NodeSelector)
	if err != nil {
		return err
	}

	err = s.pg.DB.QueryRowContext(ctx,
		`INSERT INTO tasks (task_uid, name, command, workdir, env, cpu_limit, memory_limit, timeout_sec, priority, max_retries, retry_count, retry_interval_sec, idempotency_key, node_selector, assigned_node_id, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 RETURNING id`,
		task.TaskUID, task.Name, task.Command, task.Workdir, envJSON,
		task.CPULimit, task.MemoryLimit, task.TimeoutSec, task.Priority,
		task.MaxRetries, task.RetryCount, task.RetryIntervalSec, task.IdempotencyKey,
		nodeSelectorJSON, task.AssignedNodeID,
		task.Status, task.CreatedAt, task.UpdatedAt,
	).Scan(&task.ID)
	return err
}

// Update 更新任务的所有字段，以 task_uid 匹配。
func (s *TaskStore) Update(ctx context.Context, task *model.Task) error {
	envJSON, err := json.Marshal(task.Env)
	if err != nil {
		return err
	}

	nodeSelectorJSON, err := json.Marshal(task.NodeSelector)
	if err != nil {
		return err
	}

	task.UpdatedAt = time.Now()

	_, err = s.pg.DB.ExecContext(ctx,
		`UPDATE tasks SET name=$1, command=$2, workdir=$3, env=$4, cpu_limit=$5, memory_limit=$6,
		 timeout_sec=$7, priority=$8, max_retries=$9, retry_count=$10, retry_interval_sec=$11,
		 idempotency_key=$12, node_selector=$13, assigned_node_id=$14, status=$15, exit_code=$16, stdout=$17, stderr=$18,
		 error_message=$19, pid=$20, started_at=$21, finished_at=$22, updated_at=$23
		 WHERE task_uid=$24`,
		task.Name, task.Command, task.Workdir, envJSON,
		task.CPULimit, task.MemoryLimit, task.TimeoutSec, task.Priority,
		task.MaxRetries, task.RetryCount, task.RetryIntervalSec, task.IdempotencyKey,
		nodeSelectorJSON, task.AssignedNodeID,
		task.Status, task.ExitCode, task.Stdout, task.Stderr,
		task.ErrorMessage, task.PID, task.StartedAt, task.FinishedAt,
		task.UpdatedAt, task.TaskUID,
	)
	return err
}

// GetByUID 根据 task_uid 查询单个任务。
func (s *TaskStore) GetByUID(ctx context.Context, uid string) (*model.Task, error) {
	row := s.pg.DB.QueryRowContext(ctx,
		`SELECT id, task_uid, name, command, workdir, env, cpu_limit, memory_limit,
		 timeout_sec, priority, max_retries, retry_count, retry_interval_sec, idempotency_key,
		 node_selector, assigned_node_id,
		 status, exit_code, stdout, stderr, error_message,
		 pid, started_at, finished_at, created_at, updated_at
		 FROM tasks WHERE task_uid=$1`, uid)
	return scanTask(row)
}

// List 分页查询任务列表，可按状态筛选。返回任务列表和总数。
func (s *TaskStore) List(ctx context.Context, status string, page, size int) ([]*model.Task, int, error) {
	var count int
	var countErr error
	if status == "" {
		countErr = s.pg.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&count)
	} else {
		countErr = s.pg.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE status=$1`, status).Scan(&count)
	}
	if countErr != nil {
		return nil, 0, countErr
	}

	offset := (page - 1) * size

	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.pg.DB.QueryContext(ctx,
			`SELECT id, task_uid, name, command, workdir, env, cpu_limit, memory_limit,
			 timeout_sec, priority, max_retries, retry_count, retry_interval_sec, idempotency_key,
			 node_selector, assigned_node_id,
			 status, exit_code, stdout, stderr, error_message,
			 pid, started_at, finished_at, created_at, updated_at
			 FROM tasks ORDER BY priority DESC, created_at ASC LIMIT $1 OFFSET $2`,
			size, offset)
	} else {
		rows, err = s.pg.DB.QueryContext(ctx,
			`SELECT id, task_uid, name, command, workdir, env, cpu_limit, memory_limit,
			 timeout_sec, priority, max_retries, retry_count, retry_interval_sec, idempotency_key,
			 node_selector, assigned_node_id,
			 status, exit_code, stdout, stderr, error_message,
			 pid, started_at, finished_at, created_at, updated_at
			 FROM tasks WHERE status=$1 ORDER BY priority DESC, created_at ASC LIMIT $2 OFFSET $3`,
			status, size, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task, err := scanTaskFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, task)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	return tasks, count, nil
}

// GetPendingTasks 获取所有 pending 状态的任务。
func (s *TaskStore) GetPendingTasks(ctx context.Context) ([]*model.Task, error) {
	rows, err := s.pg.DB.QueryContext(ctx,
		`SELECT id, task_uid, name, command, workdir, env, cpu_limit, memory_limit,
		 timeout_sec, priority, max_retries, retry_count, retry_interval_sec, idempotency_key,
		 node_selector, assigned_node_id,
		 status, exit_code, stdout, stderr, error_message,
		 pid, started_at, finished_at, created_at, updated_at
		 FROM tasks WHERE status=$1 ORDER BY priority DESC, created_at ASC`,
		model.TaskStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task, err := scanTaskFromRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// GetRunningTasks 获取所有 running 状态的任务。
func (s *TaskStore) GetRunningTasks(ctx context.Context) ([]*model.Task, error) {
	rows, err := s.pg.DB.QueryContext(ctx,
		`SELECT id, task_uid, name, command, workdir, env, cpu_limit, memory_limit,
		 timeout_sec, priority, max_retries, retry_count, retry_interval_sec, idempotency_key,
		 node_selector, assigned_node_id,
		 status, exit_code, stdout, stderr, error_message,
		 pid, started_at, finished_at, created_at, updated_at
		 FROM tasks WHERE status=$1 ORDER BY priority DESC, created_at ASC`,
		model.TaskStatusRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task, err := scanTaskFromRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// GetCreatedTasks 获取所有 created 状态的任务，按优先级降序、创建时间升序排列。
func (s *TaskStore) GetCreatedTasks(ctx context.Context) ([]*model.Task, error) {
	rows, err := s.pg.DB.QueryContext(ctx,
		`SELECT id, task_uid, name, command, workdir, env, cpu_limit, memory_limit,
		 timeout_sec, priority, max_retries, retry_count, retry_interval_sec, idempotency_key,
		 node_selector, assigned_node_id,
		 status, exit_code, stdout, stderr, error_message,
		 pid, started_at, finished_at, created_at, updated_at
		 FROM tasks WHERE status=$1 ORDER BY priority DESC, created_at ASC`,
		model.TaskStatusCreated)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task, err := scanTaskFromRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// scannable 接口抽象 sql.Row 和 sql.Rows 的 Scan 方法。
type scannable interface {
	Scan(dest ...any) error
}

// scanTask 从 scannable 读取一行数据并组装为 Task。
func scanTask(row scannable) (*model.Task, error) {
	task := &model.Task{}
	var envJSON []byte
	var nodeSelectorJSON []byte

	var exitCode, pid sql.NullInt64
	var startedAt, finishedAt sql.NullTime
	var stdout, stderr, errorMessage sql.NullString
	var idempotencyKey sql.NullString
	var assignedNodeID sql.NullString

	err := row.Scan(
		&task.ID, &task.TaskUID, &task.Name, &task.Command, &task.Workdir,
		&envJSON, &task.CPULimit, &task.MemoryLimit, &task.TimeoutSec,
		&task.Priority, &task.MaxRetries, &task.RetryCount, &task.RetryIntervalSec,
		&idempotencyKey,
		&nodeSelectorJSON, &assignedNodeID,
		&task.Status,
		&exitCode, &stdout, &stderr, &errorMessage,
		&pid, &startedAt, &finishedAt,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// 解析 JSONB env 字段
	if len(envJSON) > 0 {
		if err := json.Unmarshal(envJSON, &task.Env); err != nil {
			task.Env = make(map[string]string)
		}
	} else {
		task.Env = make(map[string]string)
	}

	// 解析 JSONB node_selector 字段
	if len(nodeSelectorJSON) > 0 {
		if err := json.Unmarshal(nodeSelectorJSON, &task.NodeSelector); err != nil {
			task.NodeSelector = make(map[string]string)
		}
	} else {
		task.NodeSelector = make(map[string]string)
	}

	// 处理可空字段
	if exitCode.Valid {
		task.ExitCode = new(int)
		*task.ExitCode = int(exitCode.Int64)
	}
	if pid.Valid {
		task.PID = new(int)
		*task.PID = int(pid.Int64)
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		task.FinishedAt = &finishedAt.Time
	}
	if stdout.Valid {
		task.Stdout = stdout.String
	}
	if stderr.Valid {
		task.Stderr = stderr.String
	}
	if errorMessage.Valid {
		task.ErrorMessage = errorMessage.String
	}
	if idempotencyKey.Valid {
		task.IdempotencyKey = idempotencyKey.String
	}
	if assignedNodeID.Valid {
		task.AssignedNodeID = assignedNodeID.String
	}

	return task, nil
}

// scanTaskFromRows 从 sql.Rows 读取一行数据并组装为 Task。
func scanTaskFromRows(rows *sql.Rows) (*model.Task, error) {
	return scanTask(rows)
}
