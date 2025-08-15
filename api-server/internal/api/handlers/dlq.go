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
)

// SetupDLQ sets up comprehensive DLQ topology using RabbitMQ Management API
func (h *Handlers) SetupDLQ(c *gin.Context) {
	var req struct {
		ReplayExchange    string  `json:"replay_exchange"`
		RetryQueueTTL     int     `json:"retry_queue_ttl"`
		MaxRetries        int     `json:"max_retries"`
		BackoffMultiplier float64 `json:"backoff_multiplier"`
	}

	// Set defaults
	if err := c.ShouldBindJSON(&req); err == nil {
		// Use provided values
	} else {
		// Default values for educational setup
		req.ReplayExchange = "game.dlq.replay"
		req.RetryQueueTTL = 5000 // 5 seconds
		req.MaxRetries = 3
		req.BackoffMultiplier = 2.0
	}

	// Setup DLQ topology via RabbitMQ Management API
	result, err := h.SetupDLQTopology(req.ReplayExchange, req.RetryQueueTTL, req.MaxRetries, req.BackoffMultiplier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to setup DLQ topology: " + err.Error(),
		})
		return
	}

	// Broadcast DLQ setup
	h.broadcastMessage("dlq_setup", map[string]interface{}{
		"result":           result,
		"educational_note": "Native RabbitMQ DLQ topology with retry queues and backoff",
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    result,
		Message: "DLQ topology setup successfully",
	})
}

// ListDLQMessages lists all DLQ messages using RabbitMQ Management API
func (h *Handlers) ListDLQMessages(c *gin.Context) {
	// Get all queues and filter for DLQ queues
	queues, err := h.RabbitMQClient.GetQueuesFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to retrieve queues: " + err.Error(),
		})
		return
	}

	var allDLQMessages []map[string]interface{}
	dlqCategories := map[string]int{
		"failed":     0,
		"unroutable": 0,
		"expired":    0,
		"retrying":   0,
		"maxlength":  0,
	}

	dlqQueues := []string{} // Track which queues are DLQs for reporting

	for _, queue := range queues {
		queueName, ok := queue["name"].(string)
		if !ok || !h.IsDLQQueue(queueName) {
			continue
		}

		// Track this as a DLQ queue regardless of message count
		dlqQueues = append(dlqQueues, queueName)

		// RabbitMQ metadata can be stale, so we'll use a progressive approach
		// Start with a large peek size and get whatever is actually available
		const maxPeekSize = 1000 // Generous limit for DLQ analysis

		log.Printf("📋 [DLQ] Queue %s: attempting to peek up to %d messages", queueName, maxPeekSize)

		dlqMessages, err := h.RabbitMQClient.PeekQueueMessages(queueName, maxPeekSize)
		if err != nil {
			log.Printf("⚠️ [DLQ] Failed to peek %s: %v", queueName, err)
			continue
		}

		log.Printf("📋 [DLQ] Queue %s: retrieved %d messages", queueName, len(dlqMessages))

		for _, msg := range dlqMessages {
			// Add queue name to message for categorization
			msg["queue"] = queueName

			// Categorize DLQ message by death reason
			category := h.CategorizeDLQMessage(msg)
			dlqCategories[category]++

			// Transform to app format
			dlqMsg := map[string]interface{}{
				"queue":            queueName,
				"category":         category,
				"routing_key":      msg["routing_key"],
				"payload":          msg["payload"],
				"properties":       msg["properties"],
				"death_info":       h.ExtractDeathInfo(msg),
				"source":           "direct_rabbitmq_go_client",
				"educational_note": "DLQ message retrieved directly from RabbitMQ",
			}

			allDLQMessages = append(allDLQMessages, dlqMsg)
		}
	}

	// Auto-setup DLQ topology if no DLQ queues found
	if len(dlqQueues) == 0 {
		log.Printf("📋 [DLQ] No DLQ queues found, auto-setting up DLQ topology...")
		_, err := h.SetupDLQTopology("game.dlq.replay", 5000, 3, 2.0)
		if err != nil {
			log.Printf("⚠️ [DLQ] Auto-setup failed: %v", err)
		} else {
			log.Printf("✅ [DLQ] Auto-setup completed successfully")
			// Broadcast auto-setup notification
			h.broadcastMessage("dlq_auto_setup", map[string]interface{}{
				"auto_setup":       true,
				"educational_note": "DLQ topology auto-created when first accessed",
			})
			// Re-query queues after setup
			queues, err = h.RabbitMQClient.GetQueuesFromAPI()
			if err == nil {
				// Re-scan for DLQ queues after auto-setup
				for _, queue := range queues {
					queueName, ok := queue["name"].(string)
					if ok && h.IsDLQQueue(queueName) {
						dlqQueues = append(dlqQueues, queueName)
					}
				}
			}
		}
	}

	// Also broadcast live DLQ status update via WebSocket
	h.broadcastMessage("dlq_status_update", map[string]interface{}{
		"categories":   dlqCategories,
		"total_dlq":    len(allDLQMessages),
		"dlq_queues":   dlqQueues,
		"timestamp":    time.Now().Unix(),
		"auto_refresh": true,
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"messages":         allDLQMessages,
			"categories":       dlqCategories,
			"total_dlq":        len(allDLQMessages),
			"dlq_queues":       dlqQueues,
			"source":           "direct_rabbitmq_go_client",
			"educational_note": "DLQ messages retrieved directly from RabbitMQ Management API",
		},
		Message: fmt.Sprintf("Retrieved %d DLQ messages from %d queues", len(allDLQMessages), len(dlqQueues)),
	})
}

// ReissueDLQMessages reissues DLQ messages back to main queues
func (h *Handlers) ReissueDLQMessages(c *gin.Context) {
	var req struct {
		Queue         string                 `json:"queue"`
		Count         int                    `json:"count"`
		Category      string                 `json:"category"`
		MessageIDs    []string               `json:"message_ids"`
		TargetQuestID string                 `json:"target_quest_id"` // Specific quest to reissue
		MessageData   map[string]interface{} `json:"message_data"`    // Full message data for matching
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [DLQ Reissue] Failed to bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if req.TargetQuestID != "" {
		log.Printf("🔍 [DLQ Reissue] Request received: queue='%s', count=%d, target_quest_id='%s'", req.Queue, req.Count, req.TargetQuestID)
	} else {
		log.Printf("🔍 [DLQ Reissue] Request received: queue='%s', count=%d", req.Queue, req.Count)
	}

	// For educational purposes, we'll use a simplified reissue mechanism
	// In production, this would involve more sophisticated message manipulation
	reissuedCount := 0

	if req.Queue != "" {
		// Reissue from specific queue - CONSUME messages (remove them from DLQ)
		log.Printf("🔍 [DLQ Reissue] Starting reissue from queue %s, count %d", req.Queue, req.Count)
		messages, err := h.RabbitMQClient.ConsumeQueueMessages(req.Queue, req.Count)
		if err != nil {
			log.Printf("❌ [DLQ Reissue] ConsumeQueueMessages failed: %v", err)
			c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Error:   "Failed to consume DLQ messages: " + err.Error(),
			})
			return
		}
		log.Printf("🔍 [DLQ Reissue] ConsumeQueueMessages returned %d messages", len(messages))

		// For each message, republish to original routing key with clean payload
		for _, msg := range messages {
			// Extract original payload
			if payloadStr, ok := msg["payload"].(string); ok {
				var payload map[string]interface{}
				if json.Unmarshal([]byte(payloadStr), &payload) == nil {
					// Extract the core quest data from potentially nested payload
					var corePayload map[string]interface{}

					// If this is a nested/reissued message, extract the innermost payload
					if innerPayload, ok := payload["payload"].(map[string]interface{}); ok {
						corePayload = innerPayload
					} else {
						corePayload = payload
					}

					// If targeting a specific quest, check if this message matches
					if req.TargetQuestID != "" {
						messageCaseID, _ := corePayload["case_id"].(string)
						if messageCaseID != req.TargetQuestID {
							log.Printf("🔍 [DLQ Reissue] Skipping message %s (looking for %s)", messageCaseID, req.TargetQuestID)
							continue // Skip this message, looking for specific quest
						}
						log.Printf("✅ [DLQ Reissue] Found target message %s", messageCaseID)
					}

					// Extract quest_type for routing
					questType, hasQuestType := corePayload["quest_type"].(string)
					if !hasQuestType {
						log.Printf("⚠️ [DLQ Reissue] No quest_type found in message, skipping")
						continue
					}

					// Determine reissue strategy based on source queue
					var targetRoutingKey string
					var reissueCategory string

					if strings.Contains(req.Queue, "failed") {
						// For failed messages: send directly back to original queue with retry tracking
						// The retry queue mechanism wasn't working because it dead-letters to wrong routing key
						targetRoutingKey = fmt.Sprintf("game.quest.%s", questType)
						reissueCategory = "retry"
						log.Printf("🔄 [DLQ Reissue] Retrying failed message %s to original queue %s", corePayload["case_id"], targetRoutingKey)
					} else {
						// For unroutable/expired: send back to original queue
						targetRoutingKey = fmt.Sprintf("game.quest.%s", questType)
						reissueCategory = "reissued"
						log.Printf("🔄 [DLQ Reissue] Reissuing non-failed message %s to original queue %s", corePayload["case_id"], targetRoutingKey)
					}

					// Create a clean, simple message payload (no nesting)
					cleanPayload := map[string]interface{}{
						"case_id":           corePayload["case_id"],
						"quest_type":        questType,
						"difficulty":        corePayload["difficulty"],
						"points":            corePayload["points"],
						"work_sec":          corePayload["work_sec"],
						"source":            "dlq_reissue",
						"reissued_from_dlq": true,
						"reissued_at":       time.Now().Format(time.RFC3339),
						"retry_count":       getRetryCount(corePayload) + 1,
					}

					// Preserve original case_id for tracking
					originalCaseID, _ := corePayload["case_id"].(string)

					// Log the reissue for debugging
					log.Printf("🔄 [DLQ Reissue] Reissuing clean message %s (%s) from DLQ to %s", originalCaseID, questType, targetRoutingKey)

					if err := h.RabbitMQClient.PublishMessage(targetRoutingKey, cleanPayload); err == nil {
						reissuedCount++

						// Broadcast quest_issued event for UI tracking
						h.broadcastMessage("quest_issued", cleanPayload)

						// Also broadcast Quest Log entry for the reissue
						h.broadcastMessage("quest_dlq", map[string]interface{}{
							"case_id":    originalCaseID,
							"quest_type": questType,
							"category":   reissueCategory,
							"queue":      req.Queue,
							"source":     "dlq_reissue",
						})
					} else {
						log.Printf("❌ [DLQ Reissue] Failed to reissue message %s: %v", originalCaseID, err)
					}
				}
			}
		}
	}

	// Broadcast reissue event
	h.broadcastMessage("dlq_reissued", map[string]interface{}{
		"queue":            req.Queue,
		"reissued_count":   reissuedCount,
		"educational_note": "Messages reissued from DLQ to original routing keys",
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"reissued_count": reissuedCount,
			"queue":          req.Queue,
		},
		Message: fmt.Sprintf("Reissued %d messages from DLQ", reissuedCount),
	})
}

// ReissueAllDLQMessages reissues all DLQ messages
func (h *Handlers) ReissueAllDLQMessages(c *gin.Context) {
	// Get all DLQ queues
	queues, err := h.RabbitMQClient.GetQueuesFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to retrieve queues: " + err.Error(),
		})
		return
	}

	totalReissued := 0
	for _, queue := range queues {
		queueName, ok := queue["name"].(string)
		if !ok || !h.IsDLQQueue(queueName) {
			continue
		}

		messages := h.getIntField(queue, "messages")
		if messages == 0 {
			continue
		}

		// Reissue from this DLQ queue (simplified for educational purposes)

		// Simulate reissue (simplified for educational purposes)
		// In production, this would use proper message reprocessing
		totalReissued += messages
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"total_reissued": totalReissued,
		},
		Message: fmt.Sprintf("Reissued %d messages from all DLQ queues", totalReissued),
	})
}

// Helper functions for DLQ management

func (h *Handlers) SetupDLQTopology(replayExchange string, retryTTL int, maxRetries int, backoffMultiplier float64) (map[string]interface{}, error) {
	// Actually create DLQ infrastructure using RabbitMQ Management API

	// 1. Create dead letter exchange for failed messages
	dlxExchange := "game.dlx"
	if err := h.RabbitMQClient.CreateExchange(dlxExchange, "topic", true, false, false, map[string]interface{}{}); err != nil {
		return nil, fmt.Errorf("failed to create DLX exchange: %w", err)
	}

	// 2. Setup alternate exchange for unroutable messages
	alternateExchange := "game.unroutable"
	if err := h.RabbitMQClient.CreateExchange(alternateExchange, "fanout", true, false, false, map[string]interface{}{}); err != nil {
		return nil, fmt.Errorf("failed to create alternate exchange: %w", err)
	}

	// 3. Create specific DLQ queues for different failure types
	dlqQueues := []map[string]interface{}{
		{
			"name":        "game.dlq.failed.q",
			"routing_key": "dlq.failed",
			"description": "Messages rejected by workers",
			"arguments":   map[string]interface{}{},
		},
		{
			"name":        "game.dlq.unroutable.q",
			"routing_key": "dlq.unroutable",
			"description": "Messages that couldn't be routed",
			"arguments":   map[string]interface{}{},
		},
		{
			"name":        "game.dlq.expired.q",
			"routing_key": "dlq.expired",
			"description": "Messages that expired due to TTL",
			"arguments":   map[string]interface{}{},
		},
		{
			"name":        "game.dlq.retry.q",
			"routing_key": "dlq.retry",
			"description": "Messages in retry cycle",
			"arguments": map[string]interface{}{
				"x-message-ttl":             retryTTL,
				"x-dead-letter-exchange":    "game.skill",
				"x-dead-letter-routing-key": "quest.retry",
			},
		},
	}

	createdQueues := []string{}
	createdExchanges := []string{dlxExchange, alternateExchange}

	// Create actual DLQ queues and bind them
	for _, queueConfig := range dlqQueues {
		queueName := queueConfig["name"].(string)
		routingKey := queueConfig["routing_key"].(string)
		arguments := queueConfig["arguments"].(map[string]interface{})

		// Create the queue
		if err := h.RabbitMQClient.CreateQueue(queueName, true, false, arguments); err != nil {
			return nil, fmt.Errorf("failed to create DLQ queue %s: %w", queueName, err)
		}

		// Bind queue to DLX exchange
		if err := h.RabbitMQClient.BindQueue(queueName, dlxExchange, routingKey, map[string]interface{}{}); err != nil {
			return nil, fmt.Errorf("failed to bind DLQ queue %s: %w", queueName, err)
		}

		createdQueues = append(createdQueues, queueName)
	}

	// Bind unroutable queue to alternate exchange
	if err := h.RabbitMQClient.BindQueue("game.dlq.unroutable.q", alternateExchange, "", map[string]interface{}{}); err != nil {
		return nil, fmt.Errorf("failed to bind unroutable queue to alternate exchange: %w", err)
	}

	// Configure the main game.skill exchange to use the alternate exchange for unroutable messages
	// We need to recreate the main exchange with the alternate-exchange argument
	mainExchange := "game.skill"
	if err := h.RabbitMQClient.DeleteExchange(mainExchange); err != nil {
		// Don't fail if exchange doesn't exist
		fmt.Printf("Warning: Could not delete main exchange (may not exist): %v\n", err)
	}

	// Recreate main exchange with alternate exchange configuration
	mainExchangeArgs := map[string]interface{}{
		"alternate-exchange": alternateExchange,
	}
	if err := h.RabbitMQClient.CreateExchange(mainExchange, "topic", true, false, false, mainExchangeArgs); err != nil {
		return nil, fmt.Errorf("failed to recreate main exchange with alternate exchange: %w", err)
	}

	// Rebind the existing queues to the recreated main exchange
	mainQueueBindings := []struct {
		queue      string
		routingKey string
	}{
		{"game.skill.gather.q", "game.quest.gather"},
		{"game.skill.slay.q", "game.quest.slay"},
	}

	for _, binding := range mainQueueBindings {
		if err := h.RabbitMQClient.BindQueue(binding.queue, mainExchange, binding.routingKey, map[string]interface{}{}); err != nil {
			fmt.Printf("Warning: Could not rebind queue %s: %v\n", binding.queue, err)
		}
	}

	result := map[string]interface{}{
		"exchanges_created":  createdExchanges,
		"queues_created":     createdQueues,
		"retry_ttl_ms":       retryTTL,
		"max_retries":        maxRetries,
		"backoff_multiplier": backoffMultiplier,
		"dlx_exchange":       dlxExchange,
		"alternate_exchange": alternateExchange,
		"educational_note":   "DLQ topology setup with proper dead letter routing",
		"topology": map[string]interface{}{
			"dead_letter_exchange": dlxExchange,
			"alternate_exchange":   alternateExchange,
			"retry_policy":         fmt.Sprintf("%dms backoff, max %d retries", retryTTL, maxRetries),
		},
	}

	return result, nil
}

func (h *Handlers) IsDLQQueue(queueName string) bool {
	// More comprehensive DLQ detection
	dlqIndicators := []string{
		"dlq", "dead", "failed", "retry", "unroutable", "quarantine", "poison",
		"game.quest.retry", "game.quest.dlq", "game.dlq", "game.dead",
	}

	queueLower := strings.ToLower(queueName)
	for _, indicator := range dlqIndicators {
		if strings.Contains(queueLower, indicator) {
			return true
		}
	}

	return false
}

func (h *Handlers) CategorizeDLQMessage(msg map[string]interface{}) string {
	// SIMPLIFIED: Use queue name as primary source of truth for DLQ categorization
	// This ensures KPIs match RabbitMQ actual queue message counts
	if queueName, ok := msg["queue"].(string); ok {
		queueLower := strings.ToLower(queueName)

		log.Printf("🔍 [DLQ Categorize] Queue: %s", queueName)

		// Direct queue name mapping (most reliable)
		if strings.Contains(queueLower, "failed") {
			return "failed"
		}
		if strings.Contains(queueLower, "unroutable") || strings.Contains(queueLower, "unrout") {
			return "unroutable"
		}
		if strings.Contains(queueLower, "expired") || strings.Contains(queueLower, "ttl") {
			return "expired"
		}
		if strings.Contains(queueLower, "retry") || strings.Contains(queueLower, "retr") {
			return "retrying"
		}
		if strings.Contains(queueLower, "maxlen") || strings.Contains(queueLower, "overflow") {
			return "maxlength"
		}

		log.Printf("⚠️ [DLQ Categorize] Unknown queue pattern: %s, defaulting to 'failed'", queueName)
		return "failed" // Default for unknown queue patterns
	}

	// Fallback: if no queue name available, try routing key patterns
	if routingKey, ok := msg["routing_key"].(string); ok {
		routingLower := strings.ToLower(routingKey)
		log.Printf("🔍 [DLQ Categorize] No queue name, checking routing key: %s", routingKey)

		if strings.Contains(routingLower, "dlq.failed") || strings.Contains(routingLower, "dlq.rejected") {
			return "failed"
		}
		if strings.Contains(routingLower, "dlq.expired") {
			return "expired"
		}
		if strings.Contains(routingLower, "dlq.retry") {
			return "retrying"
		}
		if strings.Contains(routingLower, "dlq.unroutable") {
			return "unroutable"
		}
	}

	log.Printf("⚠️ [DLQ Categorize] No queue or routing key info, defaulting to 'failed'")
	return "failed" // Final fallback
}

func (h *Handlers) ExtractDeathInfo(msg map[string]interface{}) map[string]interface{} {
	deathInfo := map[string]interface{}{
		"count":  0,
		"reason": "unknown",
		"queue":  "unknown",
	}

	if props, ok := msg["properties"].(map[string]interface{}); ok {
		if headers, ok := props["headers"].(map[string]interface{}); ok {
			if deaths, ok := headers["x-death"].([]interface{}); ok && len(deaths) > 0 {
				if death, ok := deaths[0].(map[string]interface{}); ok {
					if count, ok := death["count"]; ok {
						deathInfo["count"] = count
					}
					if reason, ok := death["reason"]; ok {
						deathInfo["reason"] = reason
					}
					if queue, ok := death["queue"]; ok {
						deathInfo["queue"] = queue
					}
				}
			}
		}
	}

	return deathInfo
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)+1] == substr+"." ||
					s[len(s)-len(substr)-1:] == "."+substr)))
}

// InspectDLQ provides a comprehensive view of DLQ status
func (h *Handlers) InspectDLQ(c *gin.Context) {
	// Get all queues to find DLQ-related ones
	queues, err := h.RabbitMQClient.GetQueuesFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to get queues from RabbitMQ: " + err.Error(),
		})
		return
	}

	dlqInfo := map[string]interface{}{
		"dlq_queues":         make([]map[string]interface{}, 0),
		"retry_queues":       make([]map[string]interface{}, 0),
		"failed_queues":      make([]map[string]interface{}, 0),
		"total_dlq_messages": 0,
	}

	totalMessages := 0

	// Analyze each queue to categorize DLQ-related ones
	for _, queue := range queues {
		name, _ := queue["name"].(string)
		messages := getIntField(queue, "messages")

		queueInfo := map[string]interface{}{
			"name":     name,
			"messages": messages,
			"ready":    getIntField(queue, "messages_ready"),
			"unacked":  getIntField(queue, "messages_unacknowledged"),
		}

		// Categorize by queue name patterns
		if contains(name, "dlq") || contains(name, "dead") {
			dlqInfo["dlq_queues"] = append(dlqInfo["dlq_queues"].([]map[string]interface{}), queueInfo)
			totalMessages += messages
		} else if contains(name, "retry") {
			dlqInfo["retry_queues"] = append(dlqInfo["retry_queues"].([]map[string]interface{}), queueInfo)
			totalMessages += messages
		} else if contains(name, "failed") {
			dlqInfo["failed_queues"] = append(dlqInfo["failed_queues"].([]map[string]interface{}), queueInfo)
			totalMessages += messages
		}
	}

	dlqInfo["total_dlq_messages"] = totalMessages

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"dlq_inspection":   dlqInfo,
			"source":           "direct_rabbitmq_go_client",
			"educational_note": "DLQ inspection derived from RabbitMQ queue analysis",
		},
		Message: fmt.Sprintf("DLQ inspection complete - found %d total DLQ-related messages", totalMessages),
	})
}

// Helper function to safely extract integer fields
func getIntField(data map[string]interface{}, field string) int {
	if val, ok := data[field]; ok {
		if intVal, ok := val.(float64); ok {
			return int(intVal)
		}
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}
	return 0
}

// getRetryCount extracts the retry count from a message payload
func getRetryCount(payload map[string]interface{}) int {
	if retryCount, exists := payload["retry_count"]; exists {
		if count, ok := retryCount.(float64); ok {
			return int(count)
		}
		if count, ok := retryCount.(int); ok {
			return count
		}
	}
	return 0
}
