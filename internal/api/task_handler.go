package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/service"
)

// taskService 定义任务服务接口。
type taskService interface {
	CreateTask(ctx context.Context, req service.CreateTaskRequest) (*model.Task, error)
	GetTask(ctx context.Context, uid string) (*model.Task, error)
	ListTasks(ctx context.Context, status string, page, size int) (*service.TaskListResult, error)
	CancelTask(ctx context.Context, uid string) error
	RerunTask(ctx context.Context, uid string) (*model.Task, error)
}

// createTask 处理 POST /api/v1/tasks 创建任务。
func createTask(svc taskService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.CreateTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		task, err := svc.CreateTask(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"task_uid":  task.TaskUID,
			"status":    task.Status,
			"created_at": task.CreatedAt,
		})
	}
}

// getTask 处理 GET /api/v1/tasks/:task_uid 获取单个任务。
func getTask(svc taskService) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("task_uid")

		task, err := svc.GetTask(c.Request.Context(), uid)
		if err != nil {
			if err == model.ErrTaskNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "任务未找到"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, task)
	}
}

// listTasks 处理 GET /api/v1/tasks 列出任务。
func listTasks(svc taskService) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := c.DefaultQuery("status", "")
		pageStr := c.DefaultQuery("page", "1")
		sizeStr := c.DefaultQuery("size", "10")

		pageInt, _ := strconv.Atoi(pageStr)
		if pageInt <= 0 {
			pageInt = 1
		}
		sizeInt, _ := strconv.Atoi(sizeStr)
		if sizeInt <= 0 {
			sizeInt = 10
		}

		result, err := svc.ListTasks(c.Request.Context(), status, pageInt, sizeInt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// cancelTask 处理 POST /api/v1/tasks/:task_uid/cancel 取消任务。
func cancelTask(svc taskService) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("task_uid")

		if err := svc.CancelTask(c.Request.Context(), uid); err != nil {
			if err == model.ErrTaskNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "任务未找到"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "任务已取消"})
	}
}

// rerunTask 处理 POST /api/v1/tasks/:task_uid/rerun 重跑任务。
func rerunTask(svc taskService) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("task_uid")

		task, err := svc.RerunTask(c.Request.Context(), uid)
		if err != nil {
			if err == model.ErrTaskNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "任务未找到"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"task_uid":   task.TaskUID,
			"status":     task.Status,
			"created_at": task.CreatedAt,
		})
	}
}

// getTaskLog 处理 GET /api/v1/tasks/:task_uid/log 获取任务日志。
func getTaskLog(svc taskService) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("task_uid")

		task, err := svc.GetTask(c.Request.Context(), uid)
		if err != nil {
			if err == model.ErrTaskNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "任务未找到"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"stdout": task.Stdout,
			"stderr": task.Stderr,
		})
	}
}

