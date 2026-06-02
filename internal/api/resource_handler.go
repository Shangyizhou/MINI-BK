package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/shangyizhou/mini-bk/internal/model"
)

// resourceProvider 提供资源信息（由 scheduler 实现）。
type resourceProvider interface {
	GetTotalResources() (cpu, memMB int)
}

func getResources(rp resourceProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rp == nil {
			c.JSON(http.StatusOK, gin.H{
				"cpu_cores":     0,
				"memory_mb":     0,
				"allocated_cpu": 0,
				"allocated_mem": 0,
			})
			return
		}
		totalCPU, totalMem := rp.GetTotalResources()
		c.JSON(http.StatusOK, gin.H{
			"cpu_cores": totalCPU,
			"memory_mb": totalMem,
		})
	}
}

func getStats(svc taskService, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		result, err := svc.ListTasks(ctx, "", 1, 1000)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var submitted, success, failed, running int
		for _, t := range result.Tasks {
			submitted++
			switch t.Status {
			case model.TaskStatusSuccess:
				success++
			case model.TaskStatusFailed:
				failed++
			case model.TaskStatusRunning:
				running++
			}
		}

		resp := gin.H{
			"total_tasks":  result.Total,
			"submitted":    submitted,
			"success":      success,
			"failed":       failed,
			"running":      running,
			"success_rate": safeDiv(success, submitted),
		}

		// 合并 Redis 每日统计
		if rdb != nil {
			today := time.Now().Format("2006-01-02")
			dailyStats := rdb.HGetAll(ctx, "stats:daily:"+today).Val()
			if len(dailyStats) > 0 {
				resp["daily"] = dailyStats
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

// getDailyStats 返回指定日期的每日统计。
func getDailyStats(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil {
			c.JSON(http.StatusOK, gin.H{})
			return
		}
		date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
		stats := rdb.HGetAll(c.Request.Context(), "stats:daily:"+date).Val()
		c.JSON(http.StatusOK, stats)
	}
}

func safeDiv(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
