package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

func getStats(svc taskService) gin.HandlerFunc {
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

		c.JSON(http.StatusOK, gin.H{
			"total_tasks":  result.Total,
			"submitted":    submitted,
			"success":      success,
			"failed":       failed,
			"running":      running,
			"success_rate": safeDiv(success, submitted),
		})
	}
}

func safeDiv(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
