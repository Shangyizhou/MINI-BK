package executor

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/shangyizhou/mini-bk/internal/logstream"
	"github.com/shangyizhou/mini-bk/internal/model"
)

// TaskExecutor is the interface for running tasks.
type TaskExecutor interface {
	Run(ctx context.Context, task *model.Task) *TaskResult
}

// TaskResult holds the result of a task execution.
type TaskResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
	Error    error
}

// RemoteError is an error type for task failures reported by remote agents.
type RemoteError struct {
	Message string
}

func (e *RemoteError) Error() string {
	return e.Message
}

// Executor runs tasks as OS processes with concurrency control.
type Executor struct {
	slots     chan struct{}
	logStream *logstream.LogStream
}

// NewExecutor creates a new Executor with the given maximum concurrency.
// If logStream is non-nil, task output is streamed to Redis Stream in real-time.
func NewExecutor(maxConcurrent int, logStream *logstream.LogStream) *Executor {
	return &Executor{
		slots:     make(chan struct{}, maxConcurrent),
		logStream: logStream,
	}
}

// streamOutput reads lines from a pipe, appends them to a buffer,
// and optionally streams them via logStream.
func streamOutput(ctx context.Context, pipe io.ReadCloser, buf *bytes.Buffer, logStream *logstream.LogStream, taskUID, streamName string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		// Append to buffer with newline
		buf.WriteString(line + "\n")
		// Stream to Redis Stream if available
		if logStream != nil {
			if err := logStream.Append(ctx, taskUID, line, streamName); err != nil {
				slog.Error("executor: failed to stream log line",
					"task_uid", taskUID,
					"stream", streamName,
					"error", err,
				)
			}
		}
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

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return &TaskResult{Error: err}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return &TaskResult{Error: err}
	}

	slog.Info("executor: starting task",
		"task_id", task.ID,
		"task_uid", task.TaskUID,
		"name", task.Name,
		"command", task.Command,
	)

	// Start the process
	if err := cmd.Start(); err != nil {
		return &TaskResult{Error: err}
	}

	// Read stdout and stderr concurrently
	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		streamOutput(execCtx, stdoutPipe, &stdoutBuf, e.logStream, task.TaskUID, "stdout")
	}()
	go func() {
		defer wg.Done()
		streamOutput(execCtx, stderrPipe, &stderrBuf, e.logStream, task.TaskUID, "stderr")
	}()

	// Wait for both output streams to be fully read
	wg.Wait()

	// Wait for the process to exit
	err = cmd.Wait()

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
