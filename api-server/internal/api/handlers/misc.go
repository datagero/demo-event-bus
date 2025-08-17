package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Card game handlers
func (h *Handlers) IsCardGameEnabled(c *gin.Context) {
	// For now, return a simple response
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    map[string]bool{"enabled": false},
	})
}

func (h *Handlers) GetCardGameStatus(c *gin.Context) {
	// Card game is not implemented in the Go version
	status := map[string]interface{}{
		"enabled":   false,
		"status":    "not_implemented",
		"message":   "Card game feature is not available in Go implementation",
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    status,
		Message: "Card game not implemented in Go version",
	})
}

func (h *Handlers) StartCardGame(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "start card game")
}

func (h *Handlers) StopCardGame(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "stop card game")
}

// Broker information handlers
func (h *Handlers) GetBrokerRoutes(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "broker routes")
}

func (h *Handlers) GetBrokerQueues(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "broker queues")
}

// Metrics handlers
func (h *Handlers) GetMetrics(c *gin.Context) {
	// Use native Go RabbitMQ client to derive metrics
	metrics, err := h.RabbitMQClient.DeriveMetricsFromRabbitMQ()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to derive metrics from RabbitMQ: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    metrics,
		Message: "Metrics derived natively from Go RabbitMQ client",
	})
}

func (h *Handlers) GetPlayerStats(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "player stats")
}
