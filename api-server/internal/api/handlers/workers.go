package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetWorkersStatus retrieves status of all workers from Workers client
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
		Message: "Worker status retrieved successfully",
	})
}

// StartWorker starts a new worker
func (h *Handlers) StartWorker(c *gin.Context) {
	var req struct {
		Player          string   `json:"player" binding:"required"`
		Skills          []string `json:"skills" binding:"required"`
		FailPct         float64  `json:"fail_pct"`
		SpeedMultiplier float64  `json:"speed_multiplier"`
		Workers         int      `json:"workers"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Set defaults
	if req.FailPct == 0 {
		req.FailPct = 0.2
	}
	if req.SpeedMultiplier == 0 {
		req.SpeedMultiplier = 1.0
	}
	if req.Workers == 0 {
		req.Workers = 1
	}

	// Create worker configuration
	config := map[string]interface{}{
		"player":           req.Player,
		"skills":           req.Skills,
		"fail_pct":         req.FailPct,
		"speed_multiplier": req.SpeedMultiplier,
		"workers":          req.Workers,
		"routing_mode":     "skill",
	}

	if err := h.WorkersClient.CreateWorker(config); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to start worker: " + err.Error(),
		})
		return
	}

	// Broadcast worker creation
	h.broadcastMessage("worker_created", map[string]interface{}{
		"name":             req.Player,
		"type":             "go",
		"skills":           req.Skills,
		"speed_multiplier": req.SpeedMultiplier,
		"fail_pct":         req.FailPct,
		"workers":          req.Workers,
	})

	// Also broadcast immediate roster update
	h.broadcastRosterUpdate()

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Worker started successfully",
	})
}

// StopWorker stops a worker
func (h *Handlers) StopWorker(c *gin.Context) {
	var req struct {
		Player string `json:"player" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.WorkersClient.StopWorker(req.Player); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to stop worker: " + err.Error(),
		})
		return
	}

	// Broadcast worker deletion
	h.broadcastMessage("worker_deleted", map[string]interface{}{
		"name": req.Player,
		"type": "go",
	})

	// Also broadcast immediate roster update
	h.broadcastRosterUpdate()

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Worker stopped successfully",
	})
}

// ControlWorker controls a worker (pause/resume/etc)
func (h *Handlers) ControlWorker(c *gin.Context) {
	var req struct {
		Player string `json:"player" binding:"required"`
		Action string `json:"action" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.WorkersClient.ControlWorker(req.Player, req.Action); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to control worker: " + err.Error(),
		})
		return
	}

	// Broadcast worker control
	h.broadcastMessage("worker_control", map[string]interface{}{
		"name":   req.Player,
		"action": req.Action,
		"type":   "go",
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Worker control action executed successfully",
	})
}
