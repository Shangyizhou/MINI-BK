package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shangyizhou/mini-bk/internal/model"
)

// nodeService defines the interface for node operations.
type nodeService interface {
	ListNodes(ctx context.Context, status string) ([]*model.Node, error)
	GetNode(ctx context.Context, nodeID string) (*model.Node, error)
	DrainNode(ctx context.Context, nodeID string) error
	UncordonNode(ctx context.Context, nodeID string) error
}

func listNodes(svc nodeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := c.DefaultQuery("status", "")

		nodes, err := svc.ListNodes(c.Request.Context(), status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"nodes": nodes,
			"total": len(nodes),
		})
	}
}

func getNode(svc nodeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("node_id")

		node, err := svc.GetNode(c.Request.Context(), nodeID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, node)
	}
}

func drainNode(svc nodeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("node_id")

		if err := svc.DrainNode(c.Request.Context(), nodeID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "节点已设为 Drain 状态", "node_id": nodeID})
	}
}

func uncordonNode(svc nodeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("node_id")

		if err := svc.UncordonNode(c.Request.Context(), nodeID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "节点已恢复 Online 状态", "node_id": nodeID})
	}
}
