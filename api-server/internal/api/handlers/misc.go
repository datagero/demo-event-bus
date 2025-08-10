package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RunScenario is now implemented in scenarios.go

// Card game handlers
func (h *Handlers) IsCardGameEnabled(c *gin.Context) {
	// For now, return a simple response
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    map[string]bool{"enabled": false},
	})
}

func (h *Handlers) GetCardGameStatus(c *gin.Context) {
	status, err := h.PythonClient.GetCardGameStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    status,
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
