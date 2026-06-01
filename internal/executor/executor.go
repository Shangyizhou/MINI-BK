package executor

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
)

// TaskResult holds the result of a task execution.
type TaskResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
	Error    error
}

// Executor runs tasks as OS processes with concurrency control.
type Executor struct {
	slots chan struct{}
}

// NewExecutor creates a new Executor with the given maximum concurrency.
func NewExecutor(maxConcurrent int) *Executor {
	return &Executor{
		slots: make(chan struct{}, maxConcurrent),
	}
}

// Run executes a task in a child process. It blocks until the task completes,
// times out, or the context is cancelled.
func (e *Executor) Run(ctx context.Context, task *model.Task) *TaskResult {
	// Acquire a concurrency slot
	e.slots <- struct{}{}
	defer func() { <-e.slots }()

	timeout := time.Duration(task.TimeoutSec) * time.Second
	if task.TimeoutSec <= 0 {
		timeout = 300 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", task.Command)

	// Set working directory
	if task.Workdir != "" {
		cmd.Dir = task.Workdir
	}

	// Set environment: inherit current process env + task-specific env
	cmd.Env = os.Environ()
	for k, v := range task.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	slog.Info("executor: starting task",
		"task_id", task.ID,
		"task_uid", task.TaskUID,
		"name", task.Name,
		"command", task.Command,
	)

	err := cmd.Run()

	slog.Info("executor: task finished",
		"task_id", task.ID,
		"task_uid", task.TaskUID,
		"name", task.Name,
		"command", task.Command,
	)

	result := &TaskResult{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if err != nil {
		// Check for timeout
		if execCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.Error = execCtx.Err()
			return result
		}

		// Check for cancellation
		if execCtx.Err() == context.Canceled {
			result.Error = execCtx.Err()
			return result
		}

		// Check for exit error (non-zero exit code)
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Error = exitErr
			return result
		}

		// Other errors
		result.Error = err
		return result
	}

	result.ExitCode = 0
	return result
}

