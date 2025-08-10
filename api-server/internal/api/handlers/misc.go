package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RunScenario runs an educational scenario
func (h *Handlers) RunScenario(c *gin.Context) {
	var req struct {
		Scenario string `json:"scenario" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.PythonClient.RunScenario(req.Scenario); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Scenario executed successfully",
	})
}

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
	h.delegateToTypeNotImplemented(c, "metrics")
}

func (h *Handlers) GetPlayerStats(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "player stats")
}
