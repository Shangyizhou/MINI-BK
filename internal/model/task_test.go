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
