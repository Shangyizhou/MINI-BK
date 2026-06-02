package api

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/shangyizhou/mini-bk/internal/logstream"
)

// RegisterRoutes 注册所有 API 路由。
func RegisterRoutes(r *gin.Engine, taskSvc taskService, rp resourceProvider, logStream *logstream.LogStream, rdb *redis.Client) {
	v1 := r.Group("/api/v1")
	{
		tasks := v1.Group("/tasks")
		{
			tasks.POST("", createTask(taskSvc))
			tasks.GET("", listTasks(taskSvc))
			tasks.GET("/:task_uid", getTask(taskSvc))
			tasks.POST("/:task_uid/cancel", cancelTask(taskSvc))
			tasks.POST("/:task_uid/rerun", rerunTask(taskSvc))
			tasks.GET("/:task_uid/log", getTaskLog(taskSvc))
			tasks.GET("/:task_uid/log/stream", streamTaskLog(taskSvc, logStream))
		}
		v1.GET("/resources", getResources(rp))
		v1.GET("/stats", getStats(taskSvc, rdb))
		v1.GET("/stats/daily", getDailyStats(rdb))
	}
}
