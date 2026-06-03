package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/queue"
)

// TaskStore 定义任务存储接口。
type TaskStore interface {
	Create(ctx context.Context, task *model.Task) error
	Update(ctx context.Context, task *model.Task) error
	GetByUID(ctx context.Context, uid string) (*model.Task, error)
	List(ctx context.Context, status string, page, size int) ([]*model.Task, int, error)
	GetRunningTasks(ctx context.Context) ([]*model.Task, error)
}

// CreateTaskRequest 表示创建任务的请求。
type CreateTaskRequest struct {
	Name        string            `json:"name" binding:"required"`
	Command     string            `json:"command" binding:"required"`
	Workdir     string            `json:"workdir"`
	Env         map[string]string `json:"env"`
	CPULimit    int               `json:"cpu_limit"`
	MemoryLimit int               `json:"memory_limit"`
	TimeoutSec  int               `json:"timeout_sec"`
	Priority    int               `json:"priority"`
	NodeSelector map[string]string `json:"node_selector"`
}

// TaskListResult 表示任务列表查询结果。
type TaskListResult struct {
	Tasks []*model.Task `json:"tasks"`
	Total int            `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

// TaskService 提供任务的业务逻辑操作。
type TaskService struct {
	store TaskStore
	rdb   *redis.Client
	queue queue.TaskQueue
}

// NewTaskService 创建 TaskService 实例。
func NewTaskService(store TaskStore, rdb *redis.Client, q queue.TaskQueue) *TaskService {
	return &TaskService{store: store, rdb: rdb, queue: q}
}

// CreateTask 创建一个新任务。
func (s *TaskService) CreateTask(ctx context.Context, req CreateTaskRequest) (*model.Task, error) {
	task := model.NewTask(req.Name, req.Command)

	// 应用请求中的覆盖参数
	if req.Workdir != "" {
		task.Workdir = req.Workdir
	}
	if req.Env != nil {
		task.Env = req.Env
	}
	if req.CPULimit > 0 {
		task.CPULimit = req.CPULimit
	}
	if req.MemoryLimit > 0 {
		task.MemoryLimit = req.MemoryLimit
	}
	if req.TimeoutSec > 0 {
		task.TimeoutSec = req.TimeoutSec
	}
	if req.Priority > 0 {
		task.Priority = req.Priority
	}
	if req.NodeSelector != nil {
		task.NodeSelector = req.NodeSelector
	}

	// 计算幂等键
	task.SetIdempotencyKey()

	// 通过 Redis SETNX 检查重复任务
	if s.rdb != nil {
		ok, err := s.rdb.SetNX(ctx, "tasks:dedup:"+task.IdempotencyKey, task.TaskUID, 5*time.Minute).Result()
		if err == nil && !ok {
			existingUID, _ := s.rdb.Get(ctx, "tasks:dedup:"+task.IdempotencyKey).Result()
			return nil, fmt.Errorf("duplicate task: %s", existingUID)
		}
	}

	if err := s.store.Create(ctx, task); err != nil {
		return nil, err
	}

	// 更新每日统计
	if s.rdb != nil {
		s.rdb.HIncrBy(ctx, "stats:daily:"+time.Now().Format("2006-01-02"), "submitted", 1)
	}

	// 推入任务队列
	if s.queue != nil {
		if task.Priority > 0 {
			if err := s.queue.PushPriority(ctx, task); err != nil {
				slog.Warn("task_service: failed to push task to priority queue", "error", err, "task_uid", task.TaskUID)
			}
		} else {
			if err := s.queue.Push(ctx, task); err != nil {
				slog.Warn("task_service: failed to push task to queue", "error", err, "task_uid", task.TaskUID)
			}
		}
	}

	return task, nil
}

// GetTask 根据 UID 获取任务。
func (s *TaskService) GetTask(ctx context.Context, uid string) (*model.Task, error) {
	return s.store.GetByUID(ctx, uid)
}

// ListTasks 分页列出任务。
func (s *TaskService) ListTasks(ctx context.Context, status string, page, size int) (*TaskListResult, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}

	tasks, total, err := s.store.List(ctx, status, page, size)
	if err != nil {
		return nil, err
	}

	return &TaskListResult{
		Tasks: tasks,
		Total: total,
		Page:  page,
		Size:  size,
	}, nil
}

// CancelTask 取消一个正在运行的任务。
func (s *TaskService) CancelTask(ctx context.Context, uid string) error {
	task, err := s.store.GetByUID(ctx, uid)
	if err != nil {
		return err
	}

	if task.Status.IsTerminal() {
		return errors.New("任务已处于终态，无法取消")
	}

	if err := task.TransitionTo(model.TaskStatusCanceled); err != nil {
		return err
	}

	if err := s.store.Update(ctx, task); err != nil {
		return err
	}

	// 通过 Redis Pub/Sub 通知调度器取消任务
	if s.rdb != nil {
		if err := s.rdb.Publish(ctx, "tasks:cancel:"+uid, "canceled").Err(); err != nil {
			return fmt.Errorf("publish cancel event: %w", err)
		}
	}

	return nil
}

// RerunTask 基于已有任务创建一个新任务（重跑）。
func (s *TaskService) RerunTask(ctx context.Context, uid string) (*model.Task, error) {
	original, err := s.store.GetByUID(ctx, uid)
	if err != nil {
		return nil, err
	}

	req := CreateTaskRequest{
		Name:         original.Name,
		Command:      original.Command,
		Workdir:      original.Workdir,
		Env:          original.Env,
		CPULimit:     original.CPULimit,
		MemoryLimit:  original.MemoryLimit,
		TimeoutSec:   original.TimeoutSec,
		Priority:     original.Priority,
		NodeSelector: original.NodeSelector,
	}

	return s.CreateTask(ctx, req)
}
