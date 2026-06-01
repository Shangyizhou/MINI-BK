package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/service"
)

type mockTaskService struct {
	tasks map[string]*model.Task
}

func newMockTaskService() *mockTaskService {
	return &mockTaskService{tasks: make(map[string]*model.Task)}
}

func (m *mockTaskService) CreateTask(ctx context.Context, req service.CreateTaskRequest) (*model.Task, error) {
	task := model.NewTask(req.Name, req.Command)
	task.TaskUID = "test-uid-123"
	m.tasks[task.TaskUID] = task
	return task, nil
}

func (m *mockTaskService) GetTask(ctx context.Context, uid string) (*model.Task, error) {
	t, ok := m.tasks[uid]
	if !ok {
		return nil, model.ErrTaskNotFound
	}
	return t, nil
}

func (m *mockTaskService) ListTasks(ctx context.Context, status string, page, size int) (*service.TaskListResult, error) {
	var tasks []*model.Task
	for _, t := range m.tasks {
		if status == "" || string(t.Status) == status {
			tasks = append(tasks, t)
		}
	}
	return &service.TaskListResult{Tasks: tasks, Total: len(tasks), Page: page, Size: size}, nil
}

func (m *mockTaskService) CancelTask(ctx context.Context, uid string) error {
	t, ok := m.tasks[uid]
	if !ok {
		return model.ErrTaskNotFound
	}
	t.Status = model.TaskStatusCanceled
	return nil
}

func (m *mockTaskService) RerunTask(ctx context.Context, uid string) (*model.Task, error) {
	return &model.Task{}, nil
}

func setupTestRouter(svc *mockTaskService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r, svc, nil)
	return r
}

func TestCreateTaskHandler(t *testing.T) {
	svc := newMockTaskService()
	router := setupTestRouter(svc)

	body := map[string]interface{}{
		"name":      "test-task",
		"command":   "echo hello",
		"cpu_limit": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusCreated)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["task_uid"] == nil {
		t.Error("响应中应包含 task_uid")
	}
}

func TestCreateTaskHandler_ValidationError(t *testing.T) {
	svc := newMockTaskService()
	router := setupTestRouter(svc)

	body := map[string]interface{}{
		"command": "echo hello",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetTaskHandler(t *testing.T) {
	svc := newMockTaskService()
	svc.tasks["test-uid-123"] = &model.Task{
		TaskUID: "test-uid-123",
		Name:    "test-task",
		Status:  model.TaskStatusCreated,
	}
	router := setupTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/test-uid-123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}
}

func TestGetTaskHandler_NotFound(t *testing.T) {
	svc := newMockTaskService()
	router := setupTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusNotFound)
	}
}

func TestListTasksHandler(t *testing.T) {
	svc := newMockTaskService()
	svc.tasks["uid-1"] = &model.Task{TaskUID: "uid-1", Name: "task-1", Status: model.TaskStatusCreated}
	svc.tasks["uid-2"] = &model.Task{TaskUID: "uid-2", Name: "task-2", Status: model.TaskStatusRunning}
	router := setupTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?page=1&size=10", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] == nil {
		t.Error("响应中应包含 total")
	}
}

func TestCancelTaskHandler(t *testing.T) {
	svc := newMockTaskService()
	svc.tasks["test-uid"] = &model.Task{TaskUID: "test-uid", Name: "test", Status: model.TaskStatusRunning}
	router := setupTestRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/test-uid/cancel", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusOK)
	}
}

func TestRerunTaskHandler(t *testing.T) {
	svc := newMockTaskService()
	svc.tasks["test-uid"] = &model.Task{TaskUID: "test-uid", Name: "test", Status: model.TaskStatusSuccess}
	router := setupTestRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/test-uid/rerun", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, 期望 %d", w.Code, http.StatusCreated)
	}
}
