package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetGameState retrieves the current game state
func (h *Handlers) GetGameState(c *gin.Context) {
	state, err := h.PythonClient.GetGameState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    state,
	})
}

// ResetGame resets the game state
func (h *Handlers) ResetGame(c *gin.Context) {
	// For now, delegate to Python service
	// In future phases, this could be handled by Go API server
	c.JSON(http.StatusNotImplemented, models.APIResponse{
		Success: false,
		Error:   "Reset functionality not yet migrated to Go API",
		Message: "Use Python API /api/reset for now",
	})
}

// QuickstartPlayers creates default players
func (h *Handlers) QuickstartPlayers(c *gin.Context) {
	// Delegate to Python service for now since it contains the game logic
	c.JSON(http.StatusNotImplemented, models.APIResponse{
		Success: false,
		Error:   "Quickstart functionality not yet migrated to Go API",
		Message: "Use Python API /api/players/quickstart for now",
	})
}

// StartPlayer starts a new player
func (h *Handlers) StartPlayer(c *gin.Context) {
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

	// Create worker configuration
	config := map[string]interface{}{
		"fail_pct":         req.FailPct,
		"speed_multiplier": req.SpeedMultiplier,
		"workers":          req.Workers,
	}

	if err := h.WorkersClient.StartWorker(req.Player, req.Skills, config); err != nil {
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
		Message: "Player started successfully",
	})
}

// DeletePlayer deletes a player
func (h *Handlers) DeletePlayer(c *gin.Context) {
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
			Error:   err.Error(),
		})
		return
	}

	// Broadcast roster update
	h.broadcastMessage("roster", map[string]interface{}{})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Player deleted successfully",
	})
}

// ControlPlayer controls a player (pause/resume/crash)
func (h *Handlers) ControlPlayer(c *gin.Context) {
	var req struct {
		Player string `json:"player" binding:"required"`
		Action string `json:"action" binding:"required"`
		Mode   string `json:"mode,omitempty"`
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
			Error:   err.Error(),
		})
		return
	}

	// Broadcast roster update
	h.broadcastMessage("roster", map[string]interface{}{})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Player control action executed successfully",
	})
}
