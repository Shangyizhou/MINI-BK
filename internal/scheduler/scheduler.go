package scheduler

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/queue"
)

// TaskStore defines the interface for task persistence operations used by the scheduler.
type TaskStore interface {
	GetCreatedTasks(ctx context.Context) ([]*model.Task, error)
	GetPendingTasks(ctx context.Context) ([]*model.Task, error)
	GetRunningTasks(ctx context.Context) ([]*model.Task, error)
	Update(ctx context.Context, task *model.Task) error
}

// TaskExecutor defines the interface for executing a task.
type TaskExecutor interface {
	Run(ctx context.Context, task *model.Task) *executor.TaskResult
}

const maxRetryDelay = 5 * time.Minute

// Scheduler periodically checks for tasks to schedule and dispatches them
// based on available resources.
type Scheduler struct {
	store         TaskStore
	executor      TaskExecutor
	tickInterval  time.Duration
	maxConcurrent int
	totalCPU      int
	totalMemMB    int
	rdb           *redis.Client
	cancelFuncs   map[string]context.CancelFunc // taskUID -> cancel
	cancelMu      sync.Mutex
	queue         queue.TaskQueue
}

// NewScheduler creates a new Scheduler with the given store, executor, tick interval,
// and max concurrency. It detects total CPU via runtime.NumCPU() and defaults
// totalMemMB to 8192.
func NewScheduler(store TaskStore, exec TaskExecutor, tickInterval time.Duration, maxConcurrent int, rdb *redis.Client, q queue.TaskQueue) *Scheduler {
	return &Scheduler{
		store:         store,
		executor:      exec,
		tickInterval:  tickInterval,
		maxConcurrent: maxConcurrent,
		totalCPU:      runtime.NumCPU(),
		totalMemMB:    8192,
		rdb:           rdb,
		cancelFuncs:   make(map[string]context.CancelFunc),
		queue:         q,
	}
}

// Start starts the scheduler's tick loop and listens for task cancellation
// via Redis Pub/Sub. It runs until the context is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	// Start Redis Pub/Sub listener for task cancellation
	if s.rdb != nil {
		go func() {
			pubsub := s.rdb.PSubscribe(ctx, "tasks:cancel:*")
			defer pubsub.Close()

			// Wait for subscription to be ready
			_, err := pubsub.Receive(ctx)
			if err != nil {
				slog.Error("scheduler: failed to subscribe to cancel channel", "error", err)
				return
			}

			slog.Info("scheduler: listening for task cancellation via Redis Pub/Sub")
			ch := pubsub.Channel()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						return
					}
					taskUID := strings.TrimPrefix(msg.Channel, "tasks:cancel:")
					s.cancelMu.Lock()
					if cancel, ok := s.cancelFuncs[taskUID]; ok {
						slog.Info("scheduler: cancelling task via Pub/Sub", "task_uid", taskUID)
						cancel()
						delete(s.cancelFuncs, taskUID)
					}
					s.cancelMu.Unlock()
				}
			}
		}()
	}

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler: stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick performs one scheduling cycle.
func (s *Scheduler) tick(ctx context.Context) {
	// Get currently running tasks and compute allocated resources
	runningTasks, err := s.store.GetRunningTasks(ctx)
	if err != nil {
		slog.Error("scheduler: failed to get running tasks", "error", err)
		return
	}

	allocatedCPU := 0
	allocatedMem := 0
	now := time.Now()

	for _, task := range runningTasks {
		allocatedCPU += task.CPULimit
		allocatedMem += task.MemoryLimit

		// Check for timed-out running tasks
		if task.StartedAt != nil && task.TimeoutSec > 0 {
			deadline := task.StartedAt.Add(time.Duration(task.TimeoutSec) * time.Second)
			if now.After(deadline) {
				s.failTask(ctx, task, "task timed out")
			}
		}
	}

	availableCPU := s.totalCPU - allocatedCPU
	availableMem := s.totalMemMB - allocatedMem
	availableSlots := s.maxConcurrent - len(runningTasks)

	if availableSlots <= 0 {
		return
	}

	// Pop tasks from queue and dispatch
	for availableSlots > 0 {
		task, err := s.queue.Pop(ctx, 100*time.Millisecond)
		if err != nil || task == nil {
			break // queue empty or timeout
		}

		if s.canAllocate(task, availableCPU, availableMem) {
			s.dispatch(ctx, task)
			availableCPU -= task.CPULimit
			availableMem -= task.MemoryLimit
			availableSlots--
		} else {
			// Not enough resources, push back with short delay
			_ = s.queue.PushDelayed(ctx, task, 1*time.Second)
		}
	}
}

// canAllocate checks if a task can be allocated given available resources.
func (s *Scheduler) canAllocate(task *model.Task, availableCPU, availableMem int) bool {
	if task.CPULimit > 0 && task.CPULimit > availableCPU {
		return false
	}
	if task.MemoryLimit > 0 && task.MemoryLimit > availableMem {
		return false
	}
	return true
}

// dispatch transitions a task to Running, updates the store, and launches
// execution in a goroutine.
func (s *Scheduler) dispatch(ctx context.Context, task *model.Task) {
	// If task is in created state, first transition to pending
	if task.Status == model.TaskStatusCreated {
		if err := task.TransitionTo(model.TaskStatusPending); err != nil {
			slog.Error("scheduler: failed to transition task to pending", "error", err, "task_uid", task.TaskUID)
			return
		}
		if err := s.store.Update(ctx, task); err != nil {
			slog.Error("scheduler: failed to update task", "error", err, "task_uid", task.TaskUID)
			return
		}
	}

	// Transition to running
	if err := task.TransitionTo(model.TaskStatusRunning); err != nil {
		slog.Error("scheduler: failed to transition task to running", "error", err, "task_uid", task.TaskUID)
		return
	}

	now := time.Now()
	task.StartedAt = &now

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task", "error", err, "task_uid", task.TaskUID)
		return
	}

	slog.Info("scheduler: dispatching task",
		"task_uid", task.TaskUID,
		"name", task.Name,
		"cpu_limit", task.CPULimit,
		"memory_limit", task.MemoryLimit,
	)

	// Create cancellable context for this task
	taskCtx, taskCancel := context.WithCancel(ctx)
	s.cancelMu.Lock()
	s.cancelFuncs[task.TaskUID] = taskCancel
	s.cancelMu.Unlock()

	go func(t *model.Task) {
		defer func() {
			s.cancelMu.Lock()
			delete(s.cancelFuncs, t.TaskUID)
			s.cancelMu.Unlock()
		}()
		result := s.executor.Run(taskCtx, t)
		s.completeTask(ctx, t, result)
	}(task)
}

// completeTask handles the result of a completed task execution.
func (s *Scheduler) completeTask(ctx context.Context, task *model.Task, result *executor.TaskResult) {
	task.Stdout = result.Stdout
	task.Stderr = result.Stderr

	now := time.Now()
	task.FinishedAt = &now

	if result.Error != nil {
		if result.TimedOut {
			s.failTask(ctx, task, "task timed out")
		} else {
			s.failTask(ctx, task, result.Error.Error())
		}
		return
	}

	// Set exit code
	exitCode := result.ExitCode
	task.ExitCode = &exitCode

	if err := task.TransitionTo(model.TaskStatusSuccess); err != nil {
		slog.Error("scheduler: failed to transition task to success", "error", err, "task_uid", task.TaskUID)
		return
	}

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task on success", "error", err, "task_uid", task.TaskUID)
	}

	// 更新每日成功统计
	if s.rdb != nil {
		s.rdb.HIncrBy(ctx, "stats:daily:"+time.Now().Format("2006-01-02"), "success", 1)
	}
}

// failTask transitions a task to the Failed status with an error message.
// If the task supports retry and hasn't exhausted retries, it is re-enqueued.
func (s *Scheduler) failTask(ctx context.Context, task *model.Task, errMsg string) {
	task.ErrorMessage = errMsg

	if task.CanRetry() {
		task.RetryCount++
		task.Status = model.TaskStatusPending
		task.FinishedAt = nil

		if err := s.store.Update(ctx, task); err != nil {
			slog.Error("scheduler: failed to update task for retry", "error", err, "task_uid", task.TaskUID)
			return
		}

		// 指数退避延迟并重新入队
		if s.queue != nil {
			delay := time.Duration(task.RetryIntervalSec) * time.Second * time.Duration(1<<task.RetryCount)
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			if err := s.queue.PushDelayed(ctx, task, delay); err != nil {
				slog.Error("scheduler: failed to re-enqueue task for retry", "error", err, "task_uid", task.TaskUID)
			}
		}

		slog.Info("scheduler: task scheduled for retry",
			"task_uid", task.TaskUID,
			"retry_count", task.RetryCount,
			"max_retries", task.MaxRetries,
		)
		return
	}

	// Terminal failure
	now := time.Now()
	task.FinishedAt = &now

	if err := task.TransitionTo(model.TaskStatusFailed); err != nil {
		slog.Error("scheduler: failed to transition task to failed", "error", err, "task_uid", task.TaskUID)
		return
	}

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task on failure", "error", err, "task_uid", task.TaskUID)
	}

	// 更新每日失败统计
	if s.rdb != nil {
		s.rdb.HIncrBy(ctx, "stats:daily:"+time.Now().Format("2006-01-02"), "failed", 1)
	}
}

// GetTotalResources returns the total available CPU cores and memory in MB.
func (s *Scheduler) GetTotalResources() (cpu, memMB int) {
	return s.totalCPU, s.totalMemMB
}
