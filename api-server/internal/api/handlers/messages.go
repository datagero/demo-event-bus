package handlers

import (
	"demo-event-bus-api/internal/models"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PublishMessage publishes a single message using native Go RabbitMQ client with correlation tracking
func (h *Handlers) PublishMessage(c *gin.Context) {
	var req struct {
		RoutingKey string                 `json:"routing_key" binding:"required"`
		Payload    map[string]interface{} `json:"payload" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Generate correlation ID for message tracking
	correlationID := uuid.New().String()
	timestamp := time.Now()

	// Add tracking metadata to payload
	enhancedPayload := make(map[string]interface{})
	for k, v := range req.Payload {
		enhancedPayload[k] = v
	}
	enhancedPayload["correlation_id"] = correlationID
	enhancedPayload["published_at"] = timestamp.Unix()
	enhancedPayload["published_by"] = "api_endpoint"

	// Log the publish event for traceability
	log.Printf("[%s] localhost:9000 says: Publishing message with correlation_id=%s to routing_key=%s",
		timestamp.Format(time.RFC3339), correlationID, req.RoutingKey)
	log.Printf("[%s] localhost:9000 says: Message payload: %s",
		timestamp.Format(time.RFC3339), toJSONString(enhancedPayload))

	// Use the unified publish and broadcast helper
	if err := h.publishAndBroadcast(req.RoutingKey, enhancedPayload, "api_endpoint"); err != nil {
		log.Printf("[%s] localhost:9000 says: FAILED to publish message correlation_id=%s, error=%s",
			timestamp.Format(time.RFC3339), correlationID, err.Error())
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	log.Printf("[%s] localhost:9000 says: SUCCESS published message correlation_id=%s",
		timestamp.Format(time.RFC3339), correlationID)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Message published successfully via Go RabbitMQ client",
		Data: map[string]interface{}{
			"correlation_id": correlationID,
			"routing_key":    req.RoutingKey,
			"published_at":   timestamp.Format(time.RFC3339),
		},
	})
}

// Helper function to safely convert to JSON string
func toJSONString(data interface{}) string {
	if jsonBytes, err := json.Marshal(data); err != nil {
		return fmt.Sprintf("%+v", data)
	} else {
		return string(jsonBytes)
	}
}

// PublishWave publishes multiple messages using native Go RabbitMQ client
func (h *Handlers) PublishWave(c *gin.Context) {
	var req struct {
		RoutingKey string `json:"routing_key" binding:"required"`
		Count      int    `json:"count" binding:"required"`
		Delay      int    `json:"delay"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Validate count
	if req.Count <= 0 || req.Count > 1000 {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Count must be between 1 and 1000",
		})
		return
	}

	// Publish individual messages for consistent UI broadcasting
	delay := time.Duration(req.Delay) * time.Millisecond
	for i := 0; i < req.Count; i++ {
		payload := map[string]interface{}{
			"wave_index": i + 1,
			"wave_total": req.Count,
			"case_id":    fmt.Sprintf("wave-%d-%d", time.Now().UnixNano(), i),
			"source":     "api_wave",
		}

		if err := h.publishAndBroadcast(req.RoutingKey, payload, "api_wave"); err != nil {
			c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Error:   fmt.Sprintf("Failed to publish wave message %d: %v", i+1, err),
			})
			return
		}

		// Apply delay between messages
		if delay > 0 && i < req.Count-1 {
			time.Sleep(delay)
		}
	}

	// Broadcast wave completion
	h.broadcastMessage("wave_published", map[string]interface{}{
		"routing_key": req.RoutingKey,
		"count":       req.Count,
		"delay":       req.Delay,
		"source":      "go_api_server",
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: fmt.Sprintf("Published wave of %d messages to %s", req.Count, req.RoutingKey),
	})
}

// StartMaster starts the master publisher (quest wave)
func (h *Handlers) StartMaster(c *gin.Context) {
	var req struct {
		Count int     `json:"count"`
		Delay float64 `json:"delay"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request format: " + err.Error(),
		})
		return
	}

	// Default values
	if req.Count <= 0 {
		req.Count = 10
	}
	if req.Delay <= 0 {
		req.Delay = 0.1
	}

	// Use the quest-wave scenario to publish messages
	questTypes := []string{"gather", "slay", "escort"}
	totalMessages := 0

	for _, questType := range questTypes {
		routingKey := fmt.Sprintf("game.quest.%s", questType)

		// Publish individual messages and broadcast quest_issued for each
		for i := 0; i < req.Count; i++ {
			caseID := fmt.Sprintf("wave-%s-%d-%d", questType, time.Now().UnixNano(), i)
			payload := map[string]interface{}{
				"case_id":    caseID,
				"quest_type": questType,
				"difficulty": 1.0,
				"work_sec":   2.0,
				"points":     5,
				"wave_index": i + 1,
				"wave_total": req.Count,
				"source":     "quest_wave",
			}

			// Use the unified publish and broadcast helper
			if err := h.publishAndBroadcast(routingKey, payload, "quest_wave"); err != nil {
				c.JSON(http.StatusInternalServerError, models.APIResponse{
					Success: false,
					Error:   fmt.Sprintf("Failed to publish %s message %d: %v", questType, i+1, err),
				})
				return
			}

			// Apply delay between messages
			if req.Delay > 0 && i < req.Count-1 {
				time.Sleep(time.Duration(req.Delay * float64(time.Second)))
			}
		}
		totalMessages += req.Count
	}

	// Broadcast the wave start
	h.broadcastMessage("quest_wave_started", map[string]interface{}{
		"quest_types":      questTypes,
		"count_per_type":   req.Count,
		"delay":            req.Delay,
		"total_messages":   totalMessages,
		"educational_note": "Quest wave published using Go RabbitMQ client",
	})

	// Also broadcast a dedicated quest log entry for better visibility
	h.broadcastMessage("quest_log_entry", map[string]interface{}{
		"tag_class": "info",
		"title":     "QUEST WAVE",
		"body":      fmt.Sprintf("Started wave: %d each of %s (total: %d quests, %gs delay)", req.Count, strings.Join(questTypes, ", "), totalMessages, req.Delay),
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"quest_types":    questTypes,
			"count_per_type": req.Count,
			"delay":          req.Delay,
			"total_messages": totalMessages,
			"source":         "go_api_server",
		},
		Message: fmt.Sprintf("Quest wave started: %d messages per type with %gs delay", req.Count, req.Delay),
	})
}

// SendOne sends a single quest message of the specified type
func (h *Handlers) SendOne(c *gin.Context) {
	var req struct {
		QuestType string `json:"quest_type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request format: " + err.Error(),
		})
		return
	}

	// Validate quest type
	validTypes := map[string]bool{"gather": true, "slay": true, "escort": true}
	if !validTypes[req.QuestType] {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid quest type: %s. Must be one of: gather, slay, escort", req.QuestType),
		})
		return
	}

	// Create routing key and payload
	routingKey := fmt.Sprintf("game.quest.%s", req.QuestType)
	payload := map[string]interface{}{
		"case_id":    fmt.Sprintf("single-%s-%d", req.QuestType, time.Now().UnixNano()),
		"quest_type": req.QuestType,
		"difficulty": 1.0,
		"work_sec":   2.0,
		"points":     5,
		"source":     "frontend_send_one",
	}

	// Use the unified publish and broadcast helper
	if err := h.publishAndBroadcast(routingKey, payload, "frontend_send_one"); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"quest_type":  req.QuestType,
			"routing_key": routingKey,
			"payload":     payload,
			"source":      "go_api_server",
		},
		Message: fmt.Sprintf("Single %s quest published successfully", req.QuestType),
	})
}

// ListPendingMessages retrieves pending messages directly from RabbitMQ queues
func (h *Handlers) ListPendingMessages(c *gin.Context) {
	// Get all queues from RabbitMQ
	queues, err := h.RabbitMQClient.GetQueuesFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to retrieve queues: " + err.Error(),
		})
		return
	}

	// Filter game queues and collect pending messages
	var allPendingMessages []map[string]interface{}
	totalPending := 0

	for _, queue := range queues {
		queueName, ok := queue["name"].(string)
		if !ok || !h.isGameQueue(queueName) {
			continue
		}

		ready := h.getIntField(queue, "messages_ready")
		if ready == 0 {
			continue
		}

		// Peek messages from this queue (limit 5 per queue for performance)
		messages, err := h.RabbitMQClient.PeekQueueMessages(queueName, 5)
		if err != nil {
			continue // Skip queues with errors
		}

		for _, msg := range messages {
			// Transform RabbitMQ message format to app format
			pendingMsg := map[string]interface{}{
				"queue":       queueName,
				"routing_key": msg["routing_key"],
				"payload":     msg["payload"],
				"properties":  msg["properties"],
				"status":      "pending",
				"source":      "direct_rabbitmq_go_client",
			}

			// Extract quest info if available
			if payload, ok := msg["payload"].(string); ok {
				var payloadData map[string]interface{}
				if json.Unmarshal([]byte(payload), &payloadData) == nil {
					if questId, exists := payloadData["case_id"]; exists {
						pendingMsg["quest_id"] = questId
					}
					if questType, exists := payloadData["quest_type"]; exists {
						pendingMsg["quest_type"] = questType
					}
				}
			}

			allPendingMessages = append(allPendingMessages, pendingMsg)
		}

		totalPending += ready
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"messages":         allPendingMessages,
			"total_pending":    totalPending,
			"source":           "direct_rabbitmq_go_client",
			"educational_note": "Pending messages retrieved directly from RabbitMQ queue inspection",
		},
		Message: fmt.Sprintf("Retrieved %d pending messages from %d queues", len(allPendingMessages), len(queues)),
	})
}

func (h *Handlers) ReissuePendingMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue pending messages")
}

func (h *Handlers) ReissueAllPendingMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue all pending messages")
}

func (h *Handlers) ListFailedMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "failed messages list")
}

func (h *Handlers) ReissueFailedMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue failed messages")
}

func (h *Handlers) ReissueAllFailedMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue all failed messages")
}

// DLQ handlers are now implemented in dlq.go

func (h *Handlers) ListUnroutableMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "unroutable messages list")
}

func (h *Handlers) ReissueUnroutableMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue unroutable messages")
}

func (h *Handlers) ReissueAllUnroutableMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue all unroutable messages")
}

// Helper function for not-yet-implemented endpoints
func (h *Handlers) delegateToTypeNotImplemented(c *gin.Context, functionality string) {
	c.JSON(http.StatusNotImplemented, models.APIResponse{
		Success: false,
		Error:   functionality + " functionality not yet migrated to Go API",
		Message: "Use corresponding Python API endpoint for now",
	})
}

// Helper functions for RabbitMQ data processing
func (h *Handlers) isGameQueue(queueName string) bool {
	// Check if it's a DLQ queue first - these should NOT be considered pending
	if h.IsDLQQueue(queueName) {
		return false
	}

	gameQueuePrefixes := []string{"game.", "web."}
	for _, prefix := range gameQueuePrefixes {
		if len(queueName) >= len(prefix) && queueName[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func (h *Handlers) getIntField(data map[string]interface{}, field string) int {
	if val, ok := data[field]; ok {
		if intVal, ok := val.(float64); ok {
			return int(intVal)
		}
	}
	return 0
}
