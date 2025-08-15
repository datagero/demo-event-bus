package handlers

import (
	"demo-event-bus-api/internal/models"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ReceiveWorkerEvents handles webhook events from Go workers
func (h *Handlers) ReceiveWorkerEvents(c *gin.Context) {
	var event map[string]interface{}

	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Extract event type and handle accordingly
	eventType, ok := event["type"].(string)
	if !ok {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Missing or invalid event type",
		})
		return
	}

	// Handle different event types
	switch eventType {
	case "message_event":
		h.handleMessageEvent(event)
	case "worker_status_change":
		h.handleWorkerStatusChange(event)
	case "chaos_event":
		h.handleChaosEvent(event)
	default:
		// Unknown event type, log but don't fail
		// log.Printf("Unknown event type: %s", eventType)
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Event received",
	})
}

// handleMessageEvent processes message-related events from workers
func (h *Handlers) handleMessageEvent(event map[string]interface{}) {
	// Extract event details
	eventStage, _ := event["event"].(string)
	player, _ := event["player"].(string)

	// Debug logging to trace webhook events
	log.Printf("🔔 [Webhook] Received message event: stage=%s, player=%s", eventStage, player)

	// Normalize to uppercase to accept both legacy (UPPER) and Go worker (lowercase) forms
	switch eventStage {
	case "accept":
		eventStage = "ACCEPTED"
	case "completed":
		eventStage = "COMPLETED"
	case "failed":
		eventStage = "FAILED"
	}

	// Map Go worker event names to frontend event names
	var msgType string
	switch eventStage {
	case "ACCEPTED":
		msgType = "player_accept"
		h.updatePlayerStat(player, "accepted", 1)
		h.updatePlayerStat(player, "inflight", 1)
	case "COMPLETED":
		msgType = "result_done"
		h.updatePlayerStat(player, "completed", 1)
	case "FAILED":
		msgType = "result_fail"
		h.updatePlayerStat(player, "failed", 1)
	default:
		return // Unknown stage
	}

	// Broadcast to WebSocket clients
	// The payload should contain all the message fields for the frontend to render correctly
	message, _ := event["message"].(map[string]interface{})
	if message == nil {
		message = make(map[string]interface{})
	}
	message["player"] = player    // Ensure player is in the top-level payload
	message["event"] = eventStage // Add event stage for context
	message["source"] = "go-worker-webhook"

	log.Printf("🔔 [Webhook] Broadcasting WebSocket: type=%s, player=%s, stage=%s", msgType, player, eventStage)
	h.broadcastMessage(msgType, message)
}

// handleWorkerStatusChange processes worker status change events
func (h *Handlers) handleWorkerStatusChange(event map[string]interface{}) {
	// Broadcast roster update
	h.broadcastMessage("roster", map[string]interface{}{
		"type": "go",
	})
}

// handleChaosEvent processes chaos events from workers
func (h *Handlers) handleChaosEvent(event map[string]interface{}) {
	chaosEvent, _ := event["event"].(string)
	description, _ := event["description"].(string)
	player, _ := event["player"].(string)

	var msgType string
	switch chaosEvent {
	case "disconnect":
		msgType = "chaos_disconnect"
	case "reconnect":
		msgType = "chaos_reconnect"
	case "reconnect_failed":
		msgType = "chaos_reconnect_failed"
	default:
		return // Unknown chaos event
	}

	// Broadcast chaos event to WebSocket clients
	payload := map[string]interface{}{
		"player":      player,
		"description": description,
		"source":      "go-chaos",
	}

	h.broadcastMessage(msgType, payload)
}
