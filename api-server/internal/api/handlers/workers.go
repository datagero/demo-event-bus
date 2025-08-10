package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetWorkersStatus retrieves the status of all workers
func (h *Handlers) GetWorkersStatus(c *gin.Context) {
	status, err := h.WorkersClient.GetStatus()
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

// StartWorker starts a new worker
func (h *Handlers) StartWorker(c *gin.Context) {
	var req struct {
		Name   string                 `json:"name" binding:"required"`
		Skills []string               `json:"skills" binding:"required"`
		Config map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.WorkersClient.StartWorker(req.Name, req.Skills, req.Config); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Broadcast roster update
	h.broadcastMessage("roster", map[string]interface{}{})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Worker started successfully",
	})
}

// StopWorker stops a worker
func (h *Handlers) StopWorker(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.WorkersClient.StopWorker(req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Broadcast roster update
	h.broadcastMessage("roster", map[string]interface{}{})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Worker stopped successfully",
	})
}

// ControlWorker controls a worker (pause/resume)
func (h *Handlers) ControlWorker(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Action string `json:"action" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.WorkersClient.ControlWorker(req.Name, req.Action); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Broadcast roster update
	h.broadcastMessage("roster", map[string]interface{}{})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Worker control action executed successfully",
	})
}
