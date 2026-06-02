package model

import (
	"testing"
)

func TestTaskStatusTransitions(t *testing.T) {
	tests := []struct {
		name       string
		fromStatus TaskStatus
		toStatus   TaskStatus
		wantOK     bool
	}{
		{"Created -> Pending", TaskStatusCreated, TaskStatusPending, true},
		{"Created -> Canceled", TaskStatusCreated, TaskStatusCanceled, true},
		{"Pending -> Running", TaskStatusPending, TaskStatusRunning, true},
		{"Pending -> Canceled", TaskStatusPending, TaskStatusCanceled, true},
		{"Running -> Success", TaskStatusRunning, TaskStatusSuccess, true},
		{"Running -> Failed", TaskStatusRunning, TaskStatusFailed, true},
		{"Running -> Canceled", TaskStatusRunning, TaskStatusCanceled, true},
		// 终态不可流转
		{"Success -> Running", TaskStatusSuccess, TaskStatusRunning, false},
		{"Failed -> Running", TaskStatusFailed, TaskStatusRunning, false},
		{"Canceled -> Running", TaskStatusCanceled, TaskStatusRunning, false},
		// 不可逆流
		{"Running -> Created", TaskStatusRunning, TaskStatusCreated, false},
		{"Running -> Pending", TaskStatusRunning, TaskStatusPending, false},
		{"Success -> Pending", TaskStatusSuccess, TaskStatusPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Status: tt.fromStatus}
			err := task.TransitionTo(tt.toStatus)
			if (err == nil) != tt.wantOK {
				t.Errorf("TransitionTo(%s -> %s) error = %v, wantOK = %v",
					tt.fromStatus, tt.toStatus, err, tt.wantOK)
			}
		})
	}
}

func TestTaskIsTerminal(t *testing.T) {
	terminal := []TaskStatus{TaskStatusSuccess, TaskStatusFailed, TaskStatusCanceled}
	nonTerminal := []TaskStatus{TaskStatusCreated, TaskStatusPending, TaskStatusRunning}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

func TestNewTask(t *testing.T) {
	task := NewTask("test-task", "echo hello")
	if task.TaskUID == "" {
		t.Error("TaskUID should not be empty")
	}
	if task.Status != TaskStatusCreated {
		t.Errorf("Status = %s, expected %s", task.Status, TaskStatusCreated)
	}
	if task.Workdir != "/tmp" {
		t.Errorf("Workdir = %s, expected /tmp", task.Workdir)
	}
	if task.TimeoutSec != 300 {
		t.Errorf("TimeoutSec = %d, expected 300", task.TimeoutSec)
	}
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestTaskStatusString(t *testing.T) {
	if TaskStatusCreated.String() != "created" {
		t.Errorf("TaskStatusCreated.String() = %s, expected created", TaskStatusCreated.String())
	}
	if TaskStatusRunning.String() != "running" {
		t.Errorf("TaskStatusRunning.String() = %s, expected running", TaskStatusRunning.String())
	}
}

func TestNewTaskDefaults(t *testing.T) {
	task := NewTask("test-retry", "echo hello")
	if task.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, expected 3", task.MaxRetries)
	}
	if task.RetryIntervalSec != 1 {
		t.Errorf("RetryIntervalSec = %d, expected 1", task.RetryIntervalSec)
	}
	if task.RetryCount != 0 {
		t.Errorf("RetryCount = %d, expected 0", task.RetryCount)
	}
}

func TestSetIdempotencyKey(t *testing.T) {
	task1 := NewTask("test", "echo hello")
	task1.SetIdempotencyKey()
	if task1.IdempotencyKey == "" {
		t.Fatal("IdempotencyKey should not be empty")
	}
	if len(task1.IdempotencyKey) != 16 {
		t.Errorf("IdempotencyKey length = %d, expected 16", len(task1.IdempotencyKey))
	}

	// 相同输入应生成相同 key
	task2 := NewTask("test", "echo hello")
	task2.SetIdempotencyKey()
	if task1.IdempotencyKey != task2.IdempotencyKey {
		t.Errorf("相同输入的 IdempotencyKey 应该相同: %s vs %s", task1.IdempotencyKey, task2.IdempotencyKey)
	}

	// 不同输入应生成不同 key
	task3 := NewTask("test", "echo world")
	task3.SetIdempotencyKey()
	if task1.IdempotencyKey == task3.IdempotencyKey {
		t.Errorf("不同输入的 IdempotencyKey 应该不同")
	}
}

func TestSetIdempotencyKeyWithEnv(t *testing.T) {
	task1 := NewTask("test", "echo hello")
	task1.Env["KEY"] = "VALUE"
	task1.SetIdempotencyKey()

	task2 := NewTask("test", "echo hello")
	task2.Env["KEY"] = "VALUE"
	task2.SetIdempotencyKey()

	if task1.IdempotencyKey != task2.IdempotencyKey {
		t.Errorf("相同 env 的 IdempotencyKey 应该相同")
	}

	// env 不同导致 key 不同
	task3 := NewTask("test", "echo hello")
	task3.Env["KEY"] = "OTHER"
	task3.SetIdempotencyKey()
	if task1.IdempotencyKey == task3.IdempotencyKey {
		t.Errorf("不同 env 的 IdempotencyKey 应该不同")
	}
}

func TestCanRetry(t *testing.T) {
	task := NewTask("test", "echo hello")
	if !task.CanRetry() {
		t.Error("新创建的 task 应该可以重试")
	}

	task.RetryCount = 3
	if task.CanRetry() {
		t.Error("RetryCount = MaxRetries 时不应该可以重试")
	}

	task.RetryCount = 2
	if !task.CanRetry() {
		t.Error("RetryCount < MaxRetries 时可以重试")
	}

	task.MaxRetries = 0
	task.RetryCount = 0
	if task.CanRetry() {
		t.Error("MaxRetries = 0 时不应该可以重试")
	}
}
