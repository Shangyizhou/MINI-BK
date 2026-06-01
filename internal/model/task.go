package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrTaskNotFound indicates the task was not found.
var ErrTaskNotFound = errors.New("task not found")

// TaskStatus represents the current status of a task.
type TaskStatus string

const (
	TaskStatusCreated  TaskStatus = "created"
	TaskStatusPending  TaskStatus = "pending"
	TaskStatusRunning  TaskStatus = "running"
	TaskStatusSuccess  TaskStatus = "success"
	TaskStatusFailed   TaskStatus = "failed"
	TaskStatusCanceled TaskStatus = "canceled"
)

func (s TaskStatus) String() string { return string(s) }

// IsTerminal returns true if the status is a terminal state (no further transitions allowed).
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusSuccess, TaskStatusFailed, TaskStatusCanceled:
		return true
	default:
		return false
	}
}

// validTransitions defines the allowed state machine transitions.
var validTransitions = map[TaskStatus]map[TaskStatus]bool{
	TaskStatusCreated: {
		TaskStatusPending:  true,
		TaskStatusCanceled: true,
	},
	TaskStatusPending: {
		TaskStatusRunning:  true,
		TaskStatusCanceled: true,
	},
	TaskStatusRunning: {
		TaskStatusSuccess:  true,
		TaskStatusFailed:   true,
		TaskStatusCanceled: true,
	},
	TaskStatusSuccess:  {},
	TaskStatusFailed:   {},
	TaskStatusCanceled: {},
}

// Task represents a task to be executed or already executed.
type Task struct {
	ID           int64             `json:"id"`
	TaskUID      string            `json:"task_uid"`
	Name         string            `json:"name"`
	Command      string            `json:"command"`
	Workdir      string            `json:"workdir"`
	Env          map[string]string `json:"env"`
	CPULimit     int               `json:"cpu_limit"`
	MemoryLimit  int               `json:"memory_limit"`
	TimeoutSec   int               `json:"timeout_sec"`
	Priority     int               `json:"priority"`
	Status       TaskStatus        `json:"status"`
	ExitCode     *int              `json:"exit_code"`
	Stdout       string            `json:"stdout"`
	Stderr       string            `json:"stderr"`
	ErrorMessage string            `json:"error_message"`
	PID          *int              `json:"pid"`
	StartedAt    *time.Time        `json:"started_at"`
	FinishedAt   *time.Time        `json:"finished_at"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// NewTask creates a new task with default values and a generated UUID.
func NewTask(name, command string) *Task {
	now := time.Now()
	return &Task{
		TaskUID:    uuid.New().String(),
		Name:       name,
		Command:    command,
		Workdir:    "/tmp",
		Env:        make(map[string]string),
		TimeoutSec: 300,
		Status:     TaskStatusCreated,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// TransitionTo attempts to transition the task to the target status.
// Returns an error if the transition is not allowed.
func (t *Task) TransitionTo(target TaskStatus) error {
	allowed, ok := validTransitions[t.Status]
	if !ok || !allowed[target] {
		return fmt.Errorf("invalid status transition: %s -> %s", t.Status, target)
	}
	t.Status = target
	t.UpdatedAt = time.Now()
	return nil
}
