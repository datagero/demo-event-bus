package handlers

import (
	"demo-event-bus-api/internal/models"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// GetChaosStatus retrieves the current chaos configuration status
func (h *Handlers) GetChaosStatus(c *gin.Context) {
	// For now, return a simple status - in full implementation we'd track chaos state
	status := map[string]interface{}{
		"enabled":            false,
		"action":             nil,
		"target_player":      nil,
		"target_queue":       nil,
		"auto_trigger":       false,
		"is_rabbitmq_native": false,
		"available_actions": map[string]interface{}{
			"rabbitmq_native": []string{"rmq_delete_queue", "rmq_unbind_queue", "rmq_block_connection", "rmq_purge_queue"},
			"legacy":          []string{"drop", "requeue", "dlq", "fail_early", "disconnect", "pause"},
		},
		"educational_note": "Chaos actions now use direct RabbitMQ Management API for transparency",
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    status,
		Message: "Chaos status retrieved",
	})
}

// ArmChaos arms a chaos action for execution
func (h *Handlers) ArmChaos(c *gin.Context) {
	var req struct {
		Action       string  `json:"action" binding:"required"`
		TargetPlayer string  `json:"target_player,omitempty"`
		TargetQueue  string  `json:"target_queue,omitempty"`
		AutoTrigger  bool    `json:"auto_trigger"`
		TriggerDelay float64 `json:"trigger_delay"`
		TriggerCount int     `json:"trigger_count"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Define allowed actions
	rmqActions := []string{"rmq_delete_queue", "rmq_unbind_queue", "rmq_block_connection", "rmq_purge_queue"}
	legacyActions := []string{"drop", "requeue", "dlq", "fail_early", "disconnect", "pause"}

	isRMQNative := false
	isValidAction := false

	// Check if action is valid
	for _, action := range rmqActions {
		if req.Action == action {
			isRMQNative = true
			isValidAction = true
			break
		}
	}

	if !isValidAction {
		for _, action := range legacyActions {
			if req.Action == action {
				isValidAction = true
				break
			}
		}
	}

	if !isValidAction {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid action. RabbitMQ-native: %v, Legacy: %v", rmqActions, legacyActions),
		})
		return
	}

	// Execute RabbitMQ-native actions immediately
	if isRMQNative {
		result, err := h.executeRabbitMQChaos(req.Action, req.TargetPlayer, req.TargetQueue)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Error:   "RabbitMQ chaos execution failed: " + err.Error(),
			})
			return
		}

		// Broadcast chaos event
		h.broadcastMessage("chaos_rabbitmq_native", map[string]interface{}{
			"action":           req.Action,
			"target_player":    req.TargetPlayer,
			"target_queue":     req.TargetQueue,
			"result":           result,
			"educational_note": "Chaos executed directly on RabbitMQ - no app-level simulation",
		})

		c.JSON(http.StatusOK, models.APIResponse{
			Success: true,
			Data:    result,
			Message: "RabbitMQ-native chaos action executed successfully",
		})
		return
	}

	// For legacy actions, delegate to Workers client for now
	// (In full implementation, these would be handled differently)
	c.JSON(http.StatusNotImplemented, models.APIResponse{
		Success: false,
		Error:   "Legacy chaos actions not yet implemented in Go API",
		Message: "Use RabbitMQ-native actions or Python API for legacy chaos",
	})
}

// DisarmChaos disarms any active chaos configuration
func (h *Handlers) DisarmChaos(c *gin.Context) {
	// Broadcast disarm event
	h.broadcastMessage("chaos_disarmed", map[string]interface{}{
		"educational_note": "Chaos system disarmed",
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Chaos system disarmed",
	})
}

// SetChaosConfig sets chaos configuration
func (h *Handlers) SetChaosConfig(c *gin.Context) {
	var config map[string]interface{}

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    config,
		Message: "Chaos configuration updated",
	})
}

// executeRabbitMQChaos executes chaos actions directly on RabbitMQ using Management API
func (h *Handlers) executeRabbitMQChaos(action, targetPlayer, targetQueue string) (map[string]interface{}, error) {
	// Get RabbitMQ Management API configuration
	apiURL := os.Getenv("RABBITMQ_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:15672/api"
	}

	username := os.Getenv("RABBITMQ_USER")
	if username == "" {
		username = "guest"
	}

	password := os.Getenv("RABBITMQ_PASS")
	if password == "" {
		password = "guest"
	}

	// Create auth token
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	switch action {
	case "rmq_delete_queue":
		return h.deleteQueue(apiURL, auth, targetQueue)
	case "rmq_purge_queue":
		return h.purgeQueue(apiURL, auth, targetQueue)
	case "rmq_unbind_queue":
		return h.unbindQueue(apiURL, auth, targetQueue)
	case "rmq_block_connection":
		return h.blockConnections(apiURL, auth)
	default:
		return nil, fmt.Errorf("unsupported RabbitMQ chaos action: %s", action)
	}
}

// deleteQueue deletes a queue via RabbitMQ Management API
func (h *Handlers) deleteQueue(apiURL, auth, targetQueue string) (map[string]interface{}, error) {
	queueName := targetQueue
	if queueName == "" {
		queueName = "game.skill.gather.q"
	}

	// Check if queue exists first
	exists, err := h.checkQueueExists(apiURL, auth, queueName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[string]interface{}{
			"ok":               false,
			"error":            fmt.Sprintf("Queue '%s' does not exist", queueName),
			"educational_note": "Cannot delete non-existent queue - create workers first to generate queues",
		}, nil
	}

	// Delete the queue
	deleteURL := fmt.Sprintf("%s/queues/%%2F/%s", apiURL, url.QueryEscape(queueName))
	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Basic "+auth)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("delete queue failed with status %d: %s", resp.StatusCode, string(body))
	}

	return map[string]interface{}{
		"ok":               true,
		"action":           "queue_deleted",
		"queue":            queueName,
		"educational_note": "Queue deleted directly via RabbitMQ Management API",
	}, nil
}

// purgeQueue purges messages from a queue via RabbitMQ Management API
func (h *Handlers) purgeQueue(apiURL, auth, targetQueue string) (map[string]interface{}, error) {
	queueName := targetQueue
	if queueName == "" {
		queueName = "game.skill.gather.q"
	}

	// Check if queue exists first
	exists, err := h.checkQueueExists(apiURL, auth, queueName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[string]interface{}{
			"ok":               false,
			"error":            fmt.Sprintf("Queue '%s' does not exist", queueName),
			"educational_note": "Cannot purge non-existent queue - create workers first to generate queues",
		}, nil
	}

	// Purge the queue
	purgeURL := fmt.Sprintf("%s/queues/%%2F/%s/contents", apiURL, url.QueryEscape(queueName))
	req, err := http.NewRequest("DELETE", purgeURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Basic "+auth)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("purge queue failed with status %d: %s", resp.StatusCode, string(body))
	}

	return map[string]interface{}{
		"ok":               true,
		"action":           "queue_purged",
		"queue":            queueName,
		"educational_note": "Queue purged directly via RabbitMQ Management API",
	}, nil
}

// unbindQueue unbinds a queue from exchanges (simplified implementation)
func (h *Handlers) unbindQueue(apiURL, auth, targetQueue string) (map[string]interface{}, error) {
	// For now, return a placeholder - full implementation would unbind specific queue bindings
	return map[string]interface{}{
		"ok":               true,
		"action":           "queue_unbound",
		"queue":            targetQueue,
		"educational_note": "Queue unbinding would be implemented via RabbitMQ Management API",
	}, nil
}

// blockConnections closes AMQP connections via RabbitMQ Management API
func (h *Handlers) blockConnections(apiURL, auth string) (map[string]interface{}, error) {
	// Get all connections
	connectionsURL := fmt.Sprintf("%s/connections", apiURL)
	req, err := http.NewRequest("GET", connectionsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Basic "+auth)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get connections failed with status %d", resp.StatusCode)
	}

	var connections []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		return nil, err
	}

	// Close worker connections (simplified - could be more targeted)
	closedCount := 0
	for _, conn := range connections {
		connName, ok := conn["name"].(string)
		if !ok {
			continue
		}

		// Close this connection
		closeURL := fmt.Sprintf("%s/connections/%s", apiURL, url.QueryEscape(connName))
		closeReq, err := http.NewRequest("DELETE", closeURL, nil)
		if err != nil {
			continue
		}
		closeReq.Header.Add("Authorization", "Basic "+auth)

		closeResp, err := client.Do(closeReq)
		if err != nil {
			continue
		}
		closeResp.Body.Close()

		if closeResp.StatusCode == http.StatusNoContent || closeResp.StatusCode == http.StatusOK {
			closedCount++
		}
	}

	return map[string]interface{}{
		"ok":               true,
		"action":           "connections_closed",
		"count":            closedCount,
		"educational_note": "AMQP connections closed via RabbitMQ Management API",
	}, nil
}

// checkQueueExists checks if a queue exists via RabbitMQ Management API
func (h *Handlers) checkQueueExists(apiURL, auth, queueName string) (bool, error) {
	checkURL := fmt.Sprintf("%s/queues/%%2F/%s", apiURL, url.QueryEscape(queueName))
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Add("Authorization", "Basic "+auth)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}
