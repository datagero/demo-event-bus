package handlers

import (
	"demo-event-bus-api/internal/models"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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

// ResetGame resets the game state completely
func (h *Handlers) ResetGame(c *gin.Context) {
	log.Printf("🔄 [ResetGame] Starting comprehensive reset...")

	// 0. Set reset flag to prevent ticker from repopulating roster during reset
	h.resetMu.Lock()
	h.isResetting = true
	h.resetMu.Unlock()
	defer func() {
		h.resetMu.Lock()
		h.isResetting = false
		h.resetMu.Unlock()
		log.Printf("📡 [ResetGame] Reset flag cleared - normal ticker resumed")
	}()

	var resetErrors []string
	workersStoppedCount := 0
	workersServiceAvailable := false

	// 1. Stop all Go workers with proper error handling
	workersStatus, err := h.WorkersClient.GetStatus()
	if err != nil {
		log.Printf("⚠️ [ResetGame] Workers service not accessible: %v", err)
		resetErrors = append(resetErrors, fmt.Sprintf("Workers service unavailable: %v", err))

		// Try to force close any lingering RabbitMQ connections
		log.Printf("🔌 [ResetGame] Attempting to close RabbitMQ connections as fallback...")
		h.forceCloseGameConnections()
	} else {
		workersServiceAvailable = true
		if workers, ok := workersStatus["workers"].([]interface{}); ok {
			for _, worker := range workers {
				if workerName, ok := worker.(string); ok {
					log.Printf("🛑 [ResetGame] Stopping worker: %s", workerName)
					if err := h.WorkersClient.StopWorker(workerName); err != nil {
						log.Printf("⚠️ [ResetGame] Failed to stop worker %s: %v", workerName, err)
						resetErrors = append(resetErrors, fmt.Sprintf("Failed to stop worker %s: %v", workerName, err))
					} else {
						workersStoppedCount++
						log.Printf("✅ [ResetGame] Successfully stopped worker: %s", workerName)
					}
				}
			}
		}
	}

	// 2. Delete all game-related queues and exchanges from RabbitMQ
	var deletedQueues []string
	var deletedExchanges []string

	// First get all existing queues to see what actually needs to be deleted
	existingQueues, err := h.RabbitMQClient.GetQueuesFromAPI()
	if err != nil {
		log.Printf("⚠️ [ResetGame] Warning: Could not get queue list: %v", err)
		resetErrors = append(resetErrors, fmt.Sprintf("Could not list queues: %v", err))

		// Fall back to trying to delete known game queues
		gameQueues := []string{
			"game.skill.gather.q", "game.skill.slay.q", "game.skill.escort.q",
			"game.quest.gather.q", "game.quest.slay.q", "game.quest.escort.q",
			"game.quest.gather.done.q", "game.quest.slay.done.q", "game.quest.escort.done.q",
			"game.quest.gather.fail.q", "game.quest.slay.fail.q", "game.quest.escort.fail.q",
			"game.dlq.failed.q", "game.dlq.unroutable.q", "game.dlq.expired.q", "game.dlq.retry.q",
		}
		for _, queueName := range gameQueues {
			if err := h.RabbitMQClient.DeleteQueue(queueName); err != nil {
				log.Printf("⚠️ [ResetGame] Could not delete queue %s: %v", queueName, err)
			} else {
				log.Printf("🗑️ [ResetGame] Deleted queue: %s", queueName)
				deletedQueues = append(deletedQueues, queueName)
			}
		}
	} else {
		// Delete all game-related queues that actually exist
		for _, queue := range existingQueues {
			queueName, ok := queue["name"].(string)
			if !ok {
				continue
			}

			// Delete any queue that starts with "game."
			if strings.HasPrefix(queueName, "game.") {
				if err := h.RabbitMQClient.DeleteQueue(queueName); err != nil {
					log.Printf("⚠️ [ResetGame] Could not delete queue %s: %v", queueName, err)
					resetErrors = append(resetErrors, fmt.Sprintf("Failed to delete queue %s: %v", queueName, err))
				} else {
					log.Printf("🗑️ [ResetGame] Deleted queue: %s", queueName)
					deletedQueues = append(deletedQueues, queueName)
				}
			}
		}
	}

	// 2b. Delete all game-related exchanges dynamically
	existingExchanges, err := h.RabbitMQClient.GetExchangesFromAPI()
	if err != nil {
		log.Printf("⚠️ [ResetGame] Warning: Could not get exchange list: %v", err)
		resetErrors = append(resetErrors, fmt.Sprintf("Could not list exchanges: %v", err))

		// Fall back to trying to delete known game exchanges
		gameExchanges := []string{"game.skill", "game.dlx", "game.unroutable"}
		for _, exchangeName := range gameExchanges {
			if err := h.RabbitMQClient.DeleteExchange(exchangeName); err != nil {
				log.Printf("⚠️ [ResetGame] Could not delete exchange %s: %v", exchangeName, err)
				// Exchange deletion failures are common (405 errors) so don't treat as critical errors
			} else {
				log.Printf("🗑️ [ResetGame] Deleted exchange: %s", exchangeName)
				deletedExchanges = append(deletedExchanges, exchangeName)
			}
		}
	} else {
		// Delete all game-related exchanges that actually exist
		for _, exchange := range existingExchanges {
			exchangeName, ok := exchange["name"].(string)
			if !ok {
				continue
			}

			// Delete any exchange that starts with "game." or is game-related
			if strings.HasPrefix(exchangeName, "game.") {
				if err := h.RabbitMQClient.DeleteExchange(exchangeName); err != nil {
					log.Printf("⚠️ [ResetGame] Could not delete exchange %s: %v", exchangeName, err)
					// Exchange deletion often fails with 405 Method Not Allowed for built-in exchanges
					// This is normal RabbitMQ behavior, so we log but don't treat as critical error
				} else {
					log.Printf("🗑️ [ResetGame] Deleted exchange: %s", exchangeName)
					deletedExchanges = append(deletedExchanges, exchangeName)
				}
			}
		}
	}

	// 2c. Close all game-related RabbitMQ connections and channels
	var connectionsClosedCount int
	connections, err := h.RabbitMQClient.GetConnectionsFromAPI()
	if err != nil {
		log.Printf("⚠️ [ResetGame] Warning: Could not get connections list: %v", err)
		resetErrors = append(resetErrors, fmt.Sprintf("Could not list connections: %v", err))
	} else {
		for _, conn := range connections {
			connName, ok := conn["name"].(string)
			if !ok {
				continue
			}

			// Close connections that appear to be game-related
			// This includes golang workers and any connections with game-related client properties
			shouldClose := false

			if clientProps, ok := conn["client_properties"].(map[string]interface{}); ok {
				// Check platform (golang workers)
				if platform, ok := clientProps["platform"].(string); ok && platform == "golang" {
					shouldClose = true
				}
				// Could also check for other client properties if needed
			}

			// Also close connections with specific naming patterns
			if strings.Contains(connName, "worker") || strings.Contains(connName, "game") {
				shouldClose = true
			}

			if shouldClose {
				if err := h.RabbitMQClient.CloseConnection(connName); err != nil {
					log.Printf("⚠️ [ResetGame] Could not close connection %s: %v", connName, err)
					resetErrors = append(resetErrors, fmt.Sprintf("Failed to close connection %s: %v", connName, err))
				} else {
					log.Printf("🔌 [ResetGame] Closed connection: %s", connName)
					connectionsClosedCount++
				}
			}
		}
	}

	log.Printf("🔌 [ResetGame] Closed %d RabbitMQ connections", connectionsClosedCount)

	// 3. Clear all player stats
	h.statsMu.Lock()
	h.playerStats = make(map[string]map[string]int)
	h.statsMu.Unlock()
	log.Printf("📊 [ResetGame] Cleared player stats")

	// 4. Broadcast hard reset to UI with comprehensive details
	workersStoppedStatus := "none"
	if workersServiceAvailable {
		if workersStoppedCount > 0 {
			workersStoppedStatus = fmt.Sprintf("%d", workersStoppedCount)
		} else {
			workersStoppedStatus = "0"
		}
	} else {
		workersStoppedStatus = "service_unavailable"
	}

	h.broadcastMessage("reset", map[string]interface{}{
		"soft_reset":         true, // Changed from hard_reset to avoid page reload
		"timestamp":          time.Now().Unix(),
		"source":             "go_api_server",
		"queues_deleted":     deletedQueues,
		"exchanges_deleted":  deletedExchanges,
		"connections_closed": connectionsClosedCount,
		"workers_stopped":    workersStoppedStatus,
		"workers_service":    workersServiceAvailable,
		"consumers_cleared":  true,
		"errors":             resetErrors,
	})
	log.Printf("📡 [ResetGame] Broadcast reset message to UI")

	// 5. Clear roster completely by broadcasting empty roster
	h.WSHub.BroadcastMessage(&models.WebSocketMessage{
		Type:    "roster",
		Payload: map[string]interface{}{},
		Roster:  map[string]interface{}{}, // Empty roster to clear UI
	})
	log.Printf("📡 [ResetGame] Broadcast empty roster to clear UI consumers")

	// 6. Wait for cleanup to propagate and settle
	time.Sleep(1000 * time.Millisecond) // Increased wait time

	// 6.5. Broadcast final empty roster to ensure UI is cleared (don't query RabbitMQ immediately)
	h.WSHub.BroadcastMessage(&models.WebSocketMessage{
		Type:    "roster",
		Payload: map[string]interface{}{},
		Roster:  map[string]interface{}{}, // Force empty roster
	})
	log.Printf("📡 [ResetGame] Final empty roster broadcast after cleanup")

	// 6.6. Wait a bit more for UI to update before allowing normal ticker to resume
	time.Sleep(500 * time.Millisecond)

	// 7. Determine overall success status
	hasErrors := len(resetErrors) > 0
	successMessage := fmt.Sprintf("Reset completed - %s workers stopped, %d queues deleted, %d exchanges deleted, %d connections closed, state cleared",
		workersStoppedStatus, len(deletedQueues), len(deletedExchanges), connectionsClosedCount)

	if hasErrors {
		successMessage += fmt.Sprintf(" (with %d warnings)", len(resetErrors))
		log.Printf("⚠️ [ResetGame] Reset completed with warnings: %v", resetErrors)
	} else {
		log.Printf("✅ [ResetGame] Reset completed successfully")
	}

	httpStatus := http.StatusOK
	if hasErrors && !workersServiceAvailable {
		// If workers service is completely unavailable, that's more serious
		httpStatus = http.StatusPartialContent // 206 - partial success
	}

	c.JSON(httpStatus, models.APIResponse{
		Success: true, // Still successful even with warnings
		Message: successMessage,
		Data: map[string]interface{}{
			"workers_stopped":           workersStoppedStatus,
			"workers_stopped_count":     workersStoppedCount,
			"workers_service_available": workersServiceAvailable,
			"queues_deleted":            deletedQueues,
			"queues_count":              len(deletedQueues),
			"exchanges_deleted":         deletedExchanges,
			"exchanges_count":           len(deletedExchanges),
			"connections_closed":        connectionsClosedCount,
			"stats_cleared":             true,
			"ui_reset":                  true,
			"errors":                    resetErrors,
			"has_errors":                hasErrors,
		},
	})
}

// forceCloseGameConnections attempts to close RabbitMQ connections when workers service is unavailable
// This is now a fallback function - the main reset logic handles connection closing comprehensively
func (h *Handlers) forceCloseGameConnections() {
	log.Printf("🔌 [forceCloseGameConnections] Using fallback connection cleanup...")

	// Get all RabbitMQ connections
	connections, err := h.RabbitMQClient.GetConnectionsFromAPI()
	if err != nil {
		log.Printf("⚠️ [forceCloseGameConnections] Could not get connections: %v", err)
		return
	}

	connectionsClosedCount := 0
	for _, conn := range connections {
		connName, ok := conn["name"].(string)
		if !ok {
			continue
		}

		// Check if this looks like a worker connection (golang platform)
		if clientProps, ok := conn["client_properties"].(map[string]interface{}); ok {
			if platform, ok := clientProps["platform"].(string); ok && platform == "golang" {
				// Close this connection
				if err := h.RabbitMQClient.CloseConnection(connName); err != nil {
					log.Printf("⚠️ [forceCloseGameConnections] Could not close connection %s: %v", connName, err)
				} else {
					log.Printf("🔌 [forceCloseGameConnections] Force closed connection: %s", connName)
					connectionsClosedCount++
				}
			}
		}
	}

	log.Printf("🔌 [forceCloseGameConnections] Closed %d connections", connectionsClosedCount)
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
		// Note: 0.0 is a valid failure rate, so don't override it

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

		// Also emit a frontend-compatible player_online event for immediate roster update
		h.broadcastMessage("player_online", map[string]interface{}{
			"player": player,
			"source": "go-api",
		})

		// Ensure stats entry exists so throughput/activity can start rendering
		h.ensurePlayer(player)
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

	// Set defaults (only if not explicitly set)
	// Note: 0.0 is a valid failure rate, so don't override it
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

	// Also emit a frontend-compatible player_online event so the roster updates immediately
	h.broadcastMessage("player_online", map[string]interface{}{
		"player": req.Player,
		"source": "go-api",
	})

	// Ensure stats entry exists so throughput/activity can start rendering
	h.ensurePlayer(req.Player)

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

	// Broadcast disconnect to update roster immediately in the UI
	h.broadcastMessage("player_disconnected", map[string]interface{}{
		"player": req.Player,
		"reason": "deleted",
		"source": "go-api",
	})

	// Additionally broadcast an immediate roster update with current data
	h.broadcastRosterUpdate()

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

	// Broadcast immediate roster update with current data
	h.broadcastRosterUpdate()

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Player control action executed successfully",
	})
}
