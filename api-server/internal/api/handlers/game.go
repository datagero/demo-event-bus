package handlers

import (
	"demo-event-bus-api/internal/models"
	"fmt"
	"net/http"
	"strings"

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

// QuickstartPlayers creates default players with Go workers
func (h *Handlers) QuickstartPlayers(c *gin.Context) {
	var req struct {
		Preset  string                   `json:"preset"`
		Players []map[string]interface{} `json:"players,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	var presets []map[string]interface{}

	// Define presets
	if req.Preset == "alice_bob" {
		presets = []map[string]interface{}{
			{"player": "alice", "skills": "gather,slay", "fail_pct": 0.2, "speed_multiplier": 1.0, "workers": 1},
			{"player": "bob", "skills": "slay,escort", "fail_pct": 0.1, "speed_multiplier": 0.7, "workers": 2},
		}
	} else if req.Preset == "custom" && req.Players != nil {
		presets = req.Players
	} else {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid preset. Use 'alice_bob' or 'custom' with players array",
		})
		return
	}

	// Create workers directly via Workers client
	createdCount := 0
	for _, p := range presets {
		player, _ := p["player"].(string)
		if player == "" {
			continue
		}

		skillsStr, _ := p["skills"].(string)
		skills := []string{}
		if skillsStr != "" {
			for _, skill := range strings.Split(skillsStr, ",") {
				if trimmed := strings.TrimSpace(skill); trimmed != "" {
					skills = append(skills, trimmed)
				}
			}
		}

		failPct, _ := p["fail_pct"].(float64)
		if failPct == 0 {
			failPct = 0.2
		}

		speedMultiplier, _ := p["speed_multiplier"].(float64)
		if speedMultiplier == 0 {
			speedMultiplier = 1.0
		}

		workers, _ := p["workers"].(float64)
		if workers == 0 {
			workers = 1
		}

		// Create Go worker via Workers client
		workerConfig := map[string]interface{}{
			"player":           player,
			"skills":           skills,
			"fail_pct":         failPct,
			"speed_multiplier": speedMultiplier,
			"workers":          int(workers),
			"routing_mode":     "skill",
		}

		if err := h.WorkersClient.CreateWorker(workerConfig); err != nil {
			// Log error but continue with other workers
			continue
		}

		createdCount++

		// Broadcast worker creation via WebSocket
		h.broadcastMessage("worker_created", map[string]interface{}{
			"name":             player,
			"type":             "go",
			"skills":           skills,
			"speed_multiplier": speedMultiplier,
			"fail_pct":         failPct,
			"workers":          int(workers),
		})
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"count":   createdCount,
			"created": createdCount,
		},
		Message: fmt.Sprintf("Created %d Go workers", createdCount),
	})
}

// StartPlayer starts a new player with Go worker
func (h *Handlers) StartPlayer(c *gin.Context) {
	var req struct {
		Player          string  `json:"player" binding:"required"`
		Skills          string  `json:"skills" binding:"required"`
		FailPct         float64 `json:"fail_pct"`
		SpeedMultiplier float64 `json:"speed_multiplier"`
		Workers         int     `json:"workers"`
		Prefetch        int     `json:"prefetch"`
		DropRate        float64 `json:"drop_rate"`
		SkipRate        float64 `json:"skip_rate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Parse skills
	skills := []string{}
	for _, skill := range strings.Split(req.Skills, ",") {
		if trimmed := strings.TrimSpace(skill); trimmed != "" {
			skills = append(skills, trimmed)
		}
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

	// Create Go worker via Workers client
	workerConfig := map[string]interface{}{
		"player":           req.Player,
		"skills":           skills,
		"fail_pct":         req.FailPct,
		"speed_multiplier": req.SpeedMultiplier,
		"workers":          req.Workers,
		"routing_mode":     "skill",
	}

	if err := h.WorkersClient.CreateWorker(workerConfig); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to create Go worker: " + err.Error(),
		})
		return
	}

	// Broadcast worker creation
	h.broadcastMessage("worker_created", map[string]interface{}{
		"name":             req.Player,
		"type":             "go",
		"skills":           skills,
		"speed_multiplier": req.SpeedMultiplier,
		"fail_pct":         req.FailPct,
		"workers":          req.Workers,
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: fmt.Sprintf("Created Go worker '%s' with skills %v", req.Player, skills),
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
