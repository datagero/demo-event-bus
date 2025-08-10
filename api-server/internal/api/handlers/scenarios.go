package handlers

import (
	"demo-event-bus-api/internal/models"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RunScenario runs a predefined scenario
func (h *Handlers) RunScenario(c *gin.Context) {
	var req struct {
		Scenario string                 `json:"scenario" binding:"required"`
		Params   map[string]interface{} `json:"params,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Execute scenario based on type
	var result map[string]interface{}
	var err error

	switch req.Scenario {
	case "late-bind-escort":
		result, err = h.runLateBendEscortScenario(req.Params)
	case "quest-wave":
		result, err = h.runQuestWaveScenario(req.Params)
	case "chaos-test":
		result, err = h.runChaosTestScenario(req.Params)
	case "load-test":
		result, err = h.runLoadTestScenario(req.Params)
	default:
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Unknown scenario: " + req.Scenario,
			Data: map[string]interface{}{
				"available_scenarios": []string{"late-bind-escort", "quest-wave", "chaos-test", "load-test"},
			},
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Scenario execution failed: " + err.Error(),
		})
		return
	}

	// Broadcast scenario execution
	h.broadcastMessage("scenario_executed", map[string]interface{}{
		"scenario": req.Scenario,
		"result":   result,
		"source":   "go_api_server",
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    result,
		Message: fmt.Sprintf("Scenario '%s' executed successfully", req.Scenario),
	})
}

// Scenario implementations

func (h *Handlers) runLateBendEscortScenario(params map[string]interface{}) (map[string]interface{}, error) {
	// Late-bind escort scenario: publish escort quest, then create escort worker

	// Step 1: Publish escort quest
	questPayload := map[string]interface{}{
		"case_id":    fmt.Sprintf("late-bind-%d", time.Now().UnixNano()),
		"quest_type": "escort",
		"points":     10,
		"priority":   "high",
		"scenario":   "late-bind-escort",
	}

	if err := h.RabbitMQClient.PublishMessage("game.quest.escort", questPayload); err != nil {
		return nil, fmt.Errorf("failed to publish escort quest: %w", err)
	}

	// Step 2: Wait a moment (simulating late binding)
	time.Sleep(2 * time.Second)

	// Step 3: Create escort worker
	workerConfig := map[string]interface{}{
		"player":           "late-bind-escort-worker",
		"skills":           []string{"escort"},
		"fail_pct":         0.1,
		"speed_multiplier": 1.0,
		"workers":          1,
		"routing_mode":     "skill",
	}

	if err := h.WorkersClient.CreateWorker(workerConfig); err != nil {
		return nil, fmt.Errorf("failed to create escort worker: %w", err)
	}

	return map[string]interface{}{
		"quest_published":  true,
		"worker_created":   true,
		"quest_id":         questPayload["case_id"],
		"worker_name":      "late-bind-escort-worker",
		"educational_note": "Demonstrates late-binding pattern: quest published before worker exists",
	}, nil
}

func (h *Handlers) runQuestWaveScenario(params map[string]interface{}) (map[string]interface{}, error) {
	// Quest wave scenario: publish waves of different quest types

	questTypes := []string{"gather", "slay", "escort"}
	waveSize := 5
	if size, ok := params["wave_size"].(float64); ok {
		waveSize = int(size)
	}

	totalPublished := 0
	for _, questType := range questTypes {
		routingKey := fmt.Sprintf("game.quest.%s", questType)
		if err := h.RabbitMQClient.PublishWave(routingKey, waveSize, 500*time.Millisecond); err != nil {
			return nil, fmt.Errorf("failed to publish %s wave: %w", questType, err)
		}
		totalPublished += waveSize
	}

	return map[string]interface{}{
		"waves_published":  len(questTypes),
		"total_messages":   totalPublished,
		"wave_size":        waveSize,
		"quest_types":      questTypes,
		"educational_note": "Demonstrates burst publishing across multiple quest types",
	}, nil
}

func (h *Handlers) runChaosTestScenario(params map[string]interface{}) (map[string]interface{}, error) {
	// Chaos test scenario: publish messages, then trigger chaos

	// Step 1: Publish some test messages
	testPayload := map[string]interface{}{
		"case_id":    fmt.Sprintf("chaos-test-%d", time.Now().UnixNano()),
		"quest_type": "gather",
		"points":     5,
		"scenario":   "chaos-test",
	}

	if err := h.RabbitMQClient.PublishMessage("game.quest.gather", testPayload); err != nil {
		return nil, fmt.Errorf("failed to publish test message: %w", err)
	}

	// Step 2: Wait a moment
	time.Sleep(1 * time.Second)

	// Step 3: Trigger chaos (purge queue for educational purposes)
	chaosResult, err := h.executeRabbitMQChaos("rmq_purge_queue", "", "game.skill.gather.q")
	if err != nil {
		return nil, fmt.Errorf("failed to execute chaos action: %w", err)
	}

	return map[string]interface{}{
		"test_message_published": true,
		"chaos_executed":         true,
		"chaos_result":           chaosResult,
		"educational_note":       "Demonstrates chaos engineering with message publishing",
	}, nil
}

func (h *Handlers) runLoadTestScenario(params map[string]interface{}) (map[string]interface{}, error) {
	// Load test scenario: publish high volume of messages

	messageCount := 50
	if count, ok := params["message_count"].(float64); ok {
		messageCount = int(count)
	}

	// Distribute across quest types
	questTypes := []string{"gather", "slay", "escort"}
	messagesPerType := messageCount / len(questTypes)

	totalPublished := 0
	for _, questType := range questTypes {
		routingKey := fmt.Sprintf("game.quest.%s", questType)
		if err := h.RabbitMQClient.PublishWave(routingKey, messagesPerType, 10*time.Millisecond); err != nil {
			return nil, fmt.Errorf("failed to publish %s load test: %w", questType, err)
		}
		totalPublished += messagesPerType
	}

	return map[string]interface{}{
		"total_published":   totalPublished,
		"messages_per_type": messagesPerType,
		"quest_types":       questTypes,
		"educational_note":  "Demonstrates high-volume message publishing for load testing",
	}, nil
}
