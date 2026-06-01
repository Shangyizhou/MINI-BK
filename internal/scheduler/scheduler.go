package scheduler

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/model"
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

// Scheduler periodically checks for tasks to schedule and dispatches them
// based on available resources.
type Scheduler struct {
	store         TaskStore
	executor      TaskExecutor
	tickInterval  time.Duration
	maxConcurrent int
	totalCPU      int
	totalMemMB    int
}

// NewScheduler creates a new Scheduler with the given store, executor, tick interval,
// and max concurrency. It detects total CPU via runtime.NumCPU() and defaults
// totalMemMB to 8192.
func NewScheduler(store TaskStore, exec TaskExecutor, tickInterval time.Duration, maxConcurrent int) *Scheduler {
	return &Scheduler{
		store:         store,
		executor:      exec,
		tickInterval:  tickInterval,
		maxConcurrent: maxConcurrent,
		totalCPU:      runtime.NumCPU(),
		totalMemMB:    8192,
	}
}

// Start starts the scheduler's tick loop. It runs until the context is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
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

	// Try to dispatch pending tasks first, then created tasks
	pendingTasks, err := s.store.GetPendingTasks(ctx)
	if err != nil {
		slog.Error("scheduler: failed to get pending tasks", "error", err)
		return
	}

	for _, task := range pendingTasks {
		if availableSlots <= 0 {
			break
		}
		if s.canAllocate(task, availableCPU, availableMem) {
			s.dispatch(ctx, task)
			availableCPU -= task.CPULimit
			availableMem -= task.MemoryLimit
			availableSlots--
		}
	}

	// Try created tasks
	createdTasks, err := s.store.GetCreatedTasks(ctx)
	if err != nil {
		slog.Error("scheduler: failed to get created tasks", "error", err)
		return
	}

	for _, task := range createdTasks {
		if availableSlots <= 0 {
			break
		}
		if s.canAllocate(task, availableCPU, availableMem) {
			s.dispatch(ctx, task)
			availableCPU -= task.CPULimit
			availableMem -= task.MemoryLimit
			availableSlots--
		} else {
			// Not enough resources, transition to pending
			if err := task.TransitionTo(model.TaskStatusPending); err == nil {
				_ = s.store.Update(ctx, task)
			}
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

	go func(t *model.Task) {
		result := s.executor.Run(ctx, t)
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
}

// failTask transitions a task to the Failed status with an error message.
func (s *Scheduler) failTask(ctx context.Context, task *model.Task, errMsg string) {
	task.ErrorMessage = errMsg

	now := time.Now()
	task.FinishedAt = &now

	if err := task.TransitionTo(model.TaskStatusFailed); err != nil {
		slog.Error("scheduler: failed to transition task to failed", "error", err, "task_uid", task.TaskUID)
		return
	}

	if err := s.store.Update(ctx, task); err != nil {
		slog.Error("scheduler: failed to update task on failure", "error", err, "task_uid", task.TaskUID)
	}
}

// GetTotalResources returns the total available CPU cores and memory in MB.
func (s *Scheduler) GetTotalResources() (cpu, memMB int) {
	return s.totalCPU, s.totalMemMB
}
