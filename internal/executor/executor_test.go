package executor

import (
	"context"
	"testing"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
)

func TestExecutor_RunSuccess(t *testing.T) {
	exec := NewExecutor(10)

	task := model.NewTask("test-success", "echo hello")
	task.Workdir = "/tmp"
	task.TimeoutSec = 5

	result := exec.Run(context.Background(), task)
	if result.Error != nil {
		t.Fatalf("Run() error = %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, 期望 0", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, 期望 %q", result.Stdout, "hello\n")
	}
}

func TestExecutor_RunTimeout(t *testing.T) {
	exec := NewExecutor(10)

	task := model.NewTask("test-timeout", "sleep 10")
	task.Workdir = "/tmp"
	task.TimeoutSec = 1

	result := exec.Run(context.Background(), task)
	if result.Error == nil {
		t.Fatal("Run() 应该有错误（超时）")
	}
	if !result.TimedOut {
		t.Error("TimedOut 应该为 true")
	}
}

func TestExecutor_RunFailedCommand(t *testing.T) {
	exec := NewExecutor(10)

	task := model.NewTask("test-fail", "exit 42")
	task.Workdir = "/tmp"
	task.TimeoutSec = 5

	result := exec.Run(context.Background(), task)
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, 期望 42", result.ExitCode)
	}
}

func TestExecutor_RunWithEnv(t *testing.T) {
	exec := NewExecutor(10)

	task := model.NewTask("test-env", "echo $MY_VAR")
	task.Workdir = "/tmp"
	task.TimeoutSec = 5
	task.Env = map[string]string{"MY_VAR": "my_value"}

	result := exec.Run(context.Background(), task)
	if result.Error != nil {
		t.Fatalf("Run() error = %v", result.Error)
	}
	if result.Stdout != "my_value\n" {
		t.Errorf("Stdout = %q, 期望 %q", result.Stdout, "my_value\n")
	}
}

func TestExecutor_Cancel(t *testing.T) {
	exec := NewExecutor(10)

	task := model.NewTask("test-cancel", "sleep 60")
	task.Workdir = "/tmp"
	task.TimeoutSec = 120

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan *TaskResult, 1)
	go func() {
		resultCh <- exec.Run(ctx, task)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case result := <-resultCh:
		if result.Error == nil {
			t.Error("Run() 应该有错误（被取消）")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("等待取消超时")
	}
}

func TestExecutor_ConcurrencyLimit(t *testing.T) {
	maxConcurrent := 2
	exec := NewExecutor(maxConcurrent)

	started := make(chan struct{}, 5)

	for i := 0; i < 5; i++ {
		go func() {
			task := model.NewTask("test-concurrency", "sleep 2")
			task.Workdir = "/tmp"
			task.TimeoutSec = 10
			started <- struct{}{}
			exec.Run(context.Background(), task)
		}()
	}

	time.Sleep(200 * time.Millisecond)
	if len(started) < 2 {
		t.Errorf("预期至少 2 个任务快速启动，实际 %d", len(started))
	}
}
