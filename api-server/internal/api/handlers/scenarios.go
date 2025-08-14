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

	// Reset application state before running scenario
	if err := h.resetApplicationState(); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to reset application state: " + err.Error(),
		})
		return
	}

	// Execute scenario based on type
	var result map[string]interface{}
	var err error

	switch req.Scenario {
	case "late-bind-escort":
		result, err = h.RunLateBendEscortScenario(req.Params)
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

	// Broadcast scenario completion for UI updates
	h.broadcastMessage("scenario_complete", map[string]interface{}{
		"scenario":  req.Scenario,
		"success":   true,
		"timestamp": time.Now().Unix(),
		"message":   fmt.Sprintf("Scenario '%s' completed successfully", req.Scenario),
	})

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    result,
		Message: fmt.Sprintf("Scenario '%s' executed successfully with clean state reset", req.Scenario),
	})
}

// resetApplicationState clears all workers and queues for a clean scenario start
func (h *Handlers) resetApplicationState() error {
	// 1. Stop all workers
	workersStatus, err := h.WorkersClient.GetStatus()
	if err == nil {
		if workers, ok := workersStatus["workers"].([]interface{}); ok {
			for _, worker := range workers {
				if workerName, ok := worker.(string); ok {
					h.WorkersClient.StopWorker(workerName)
				}
			}
		}
	}

	// 2. Purge all game-related queues
	gameQueues := []string{
		"game.skill.gather.q",
		"game.skill.slay.q",
		"game.skill.escort.q",
		"game.quest.gather.q",
		"game.quest.slay.q",
		"game.quest.escort.q",
	}

	for _, queueName := range gameQueues {
		h.RabbitMQClient.PurgeQueue(queueName)
	}

	// 3. Clear player stats for clean UI
	h.statsMu.Lock()
	h.playerStats = make(map[string]map[string]int)
	h.statsMu.Unlock()

	// 4. Broadcast complete UI reset
	h.broadcastMessage("scenario_reset", map[string]interface{}{
		"action":          "full_reset",
		"queues_purged":   gameQueues,
		"workers_stopped": "all",
		"ui_reset":        true,
		"timestamp":       time.Now().Unix(),
	})

	// Wait a moment for cleanup to complete
	time.Sleep(500 * time.Millisecond)

	return nil
}

// Scenario implementations

func (h *Handlers) RunLateBendEscortScenario(params map[string]interface{}) (map[string]interface{}, error) {
	// Late-bind escort (backlog handoff) scenario:
	// This scenario demonstrates what happens when messages are published before a queue exists,
	// and how a shared skill queue enables handoff between workers.

	startTime := time.Now()
	timeline := []map[string]interface{}{}

	// Extract parameters with defaults
	backlogMessages := 3
	if val, ok := params["backlog_messages"].(float64); ok {
		backlogMessages = int(val)
	}

	_ = 3 // delay_seconds parameter available but not used in this improved scenario
	if val, ok := params["delay_seconds"].(float64); ok {
		_ = int(val) // Available for future use if timing needs adjustment
	}

	workerPrefix := "escort"
	if val, ok := params["worker_prefix"].(string); ok {
		workerPrefix = val
	}

	scenarioID := fmt.Sprintf("%s-scenario-%d", workerPrefix, time.Now().UnixNano())
	bobWorkerName := fmt.Sprintf("bob-escort-guard-%s", scenarioID)
	daveWorkerName := fmt.Sprintf("dave-escort-backup-%s", scenarioID)

	// ===== PHASE 1: Publish escort quests while no escort queue exists =====
	timeline = append(timeline, map[string]interface{}{
		"phase":     "1-unroutable",
		"timestamp": time.Now().Unix(),
		"action":    "Publishing escort quests before queue exists (will be dropped)",
	})

	// Send initial messages that will be unroutable and dropped
	for i := 0; i < backlogMessages; i++ {
		questPayload := map[string]interface{}{
			"case_id":    fmt.Sprintf("%s-unroutable-%d", scenarioID, i),
			"quest_type": "escort",
			"points":     10,
			"difficulty": 1.0,
			"work_sec":   3.0, // Reasonable work time for visibility
			"scenario":   "late-bind-escort-unroutable",
		}

		if err := h.publishAndBroadcast("game.skill.escort", questPayload, "late-bind-escort-scenario"); err != nil {
			return nil, fmt.Errorf("phase 1 failed - publish unroutable message %d: %w", i, err)
		}
	}

	timeline = append(timeline, map[string]interface{}{
		"phase":     "1-unroutable",
		"timestamp": time.Now().Unix(),
		"action":    fmt.Sprintf("Published %d unroutable escort quests (dropped by broker)", backlogMessages),
	})

	// Wait a moment to demonstrate unroutable messages
	time.Sleep(2 * time.Second)

	// ===== PHASE 2: Bob arrives and declares/binds game.skill.escort.q =====
	timeline = append(timeline, map[string]interface{}{
		"phase":     "2-bob-arrives",
		"timestamp": time.Now().Unix(),
		"action":    "Bob (escort guard) arrives and declares escort queue",
	})

	// Create Bob - the first escort worker who declares the queue
	bobWorkerConfig := map[string]interface{}{
		"player":           bobWorkerName,
		"skills":           []string{"escort"},
		"fail_pct":         0.0, // No failures for predictable testing
		"speed_multiplier": 3.0, // Moderate processing speed
		"workers":          1,
	}

	if err := h.WorkersClient.CreateWorker(bobWorkerConfig); err != nil {
		return nil, fmt.Errorf("phase 2 failed - create Bob worker: %w", err)
	}

	// Wait for Bob to connect and declare queue
	time.Sleep(1 * time.Second)

	// ===== PHASE 3: Publish more escort quests → these are now queued and processed =====
	timeline = append(timeline, map[string]interface{}{
		"phase":     "3-routable",
		"timestamp": time.Now().Unix(),
		"action":    "Publishing escort quests now that queue exists (will be processed)",
	})

	// Send messages that are now routable and processed by Bob
	for i := 0; i < backlogMessages; i++ {
		questPayload := map[string]interface{}{
			"case_id":    fmt.Sprintf("%s-routable-%d", scenarioID, i),
			"quest_type": "escort",
			"points":     10,
			"difficulty": 1.0,
			"work_sec":   3.0,
			"scenario":   "late-bind-escort-routable",
		}

		if err := h.publishAndBroadcast("game.skill.escort", questPayload, "late-bind-escort-scenario"); err != nil {
			return nil, fmt.Errorf("phase 3 failed - publish routable message %d: %w", i, err)
		}
	}

	timeline = append(timeline, map[string]interface{}{
		"phase":     "3-routable",
		"timestamp": time.Now().Unix(),
		"action":    fmt.Sprintf("Published %d routable escort quests (being processed by Bob)", backlogMessages),
	})

	// Wait for Bob to process some messages
	time.Sleep(2 * time.Second)

	// ===== PHASE 4: Pause Bob → publish more → messages pile up as Ready in escort queue =====
	timeline = append(timeline, map[string]interface{}{
		"phase":     "4-pause-bob",
		"timestamp": time.Now().Unix(),
		"action":    "Pausing Bob to create backlog scenario",
	})

	// Pause Bob to create a backlog
	if err := h.WorkersClient.ControlWorker(bobWorkerName, "pause"); err != nil {
		return nil, fmt.Errorf("phase 4 failed - pause Bob: %w", err)
	}

	// Wait for pause to take effect
	time.Sleep(1 * time.Second)

	// Publish messages while Bob is paused to create backlog
	for i := 0; i < backlogMessages; i++ {
		questPayload := map[string]interface{}{
			"case_id":    fmt.Sprintf("%s-backlog-%d", scenarioID, i),
			"quest_type": "escort",
			"points":     15,
			"difficulty": 1.5,
			"work_sec":   3.0,
			"scenario":   "late-bind-escort-backlog",
		}

		if err := h.publishAndBroadcast("game.skill.escort", questPayload, "late-bind-escort-scenario"); err != nil {
			return nil, fmt.Errorf("phase 4 failed - publish backlog message %d: %w", i, err)
		}
	}

	// Check queue state with backlog
	queueInfo, err := h.RabbitMQClient.GetQueueInfo("game.skill.escort.q")
	backlogCount := 0
	if err == nil {
		if messages, ok := queueInfo["messages"].(int); ok {
			backlogCount = messages
		} else if messages, ok := queueInfo["messages"].(float64); ok {
			backlogCount = int(messages)
		}
	}

	timeline = append(timeline, map[string]interface{}{
		"phase":         "4-pause-bob",
		"timestamp":     time.Now().Unix(),
		"action":        fmt.Sprintf("Bob paused, %d messages piling up in Ready state", backlogCount),
		"backlog_count": backlogCount,
	})

	// ===== PHASE 5: Dave arrives (also escort) → consumes from same queue =====
	timeline = append(timeline, map[string]interface{}{
		"phase":     "5-dave-arrives",
		"timestamp": time.Now().Unix(),
		"action":    "Dave (escort backup) arrives to drain backlog and help with new messages",
	})

	// Create Dave - the backup escort worker
	daveWorkerConfig := map[string]interface{}{
		"player":           daveWorkerName,
		"skills":           []string{"escort"},
		"fail_pct":         0.0,
		"speed_multiplier": 4.0, // Slightly faster than Bob
		"workers":          1,
	}

	if err := h.WorkersClient.CreateWorker(daveWorkerConfig); err != nil {
		return nil, fmt.Errorf("phase 5 failed - create Dave worker: %w", err)
	}

	// Wait for Dave to start consuming backlog
	time.Sleep(2 * time.Second)

	// Publish a few more messages while Dave is working
	for i := 0; i < 2; i++ {
		questPayload := map[string]interface{}{
			"case_id":    fmt.Sprintf("%s-handoff-%d", scenarioID, i),
			"quest_type": "escort",
			"points":     20,
			"difficulty": 2.0,
			"work_sec":   3.0,
			"scenario":   "late-bind-escort-handoff",
		}

		if err := h.publishAndBroadcast("game.skill.escort", questPayload, "late-bind-escort-scenario"); err != nil {
			return nil, fmt.Errorf("phase 5 failed - publish handoff message %d: %w", i, err)
		}
	}

	// Wait for final consumption
	time.Sleep(3 * time.Second)

	// Check final queue state
	finalQueueInfo, err := h.RabbitMQClient.GetQueueInfo("game.skill.escort.q")
	finalCount := 0
	if err == nil {
		if messages, ok := finalQueueInfo["messages"].(int); ok {
			finalCount = messages
		} else if messages, ok := finalQueueInfo["messages"].(float64); ok {
			finalCount = int(messages)
		}
	}

	timeline = append(timeline, map[string]interface{}{
		"phase":       "5-dave-arrives",
		"timestamp":   time.Now().Unix(),
		"action":      "Backlog handoff completed - Dave draining queue while Bob is paused",
		"final_count": finalCount,
		"consumed":    backlogCount - finalCount,
	})

	executionTime := time.Since(startTime)
	backlogConsumed := finalCount < backlogCount

	return map[string]interface{}{
		"phase_1_complete":  true, // Unroutable messages published
		"phase_2_complete":  true, // Bob declares queue
		"phase_3_complete":  true, // Routable messages processed
		"phase_4_complete":  true, // Bob paused, backlog created
		"phase_5_complete":  true, // Dave arrives and drains backlog
		"backlog_consumed":  backlogConsumed,
		"scenario_id":       scenarioID,
		"bob_worker":        bobWorkerName,
		"dave_worker":       daveWorkerName,
		"backlog_messages":  backlogMessages,
		"peak_backlog":      backlogCount,
		"final_backlog":     finalCount,
		"messages_consumed": backlogCount - finalCount,
		"execution_time_ms": executionTime.Milliseconds(),
		"timeline":          timeline,
		"educational_note":  "Demonstrates late-bind escort (backlog handoff): unroutable messages → queue creation → pause/backlog → shared queue handoff between workers",
		"educational_concepts": []string{
			"Unroutable messages are dropped when no queue exists",
			"First worker declares and binds the queue",
			"Queue enables message persistence and handoff",
			"Multiple workers can share the same skill queue",
			"Paused workers create visible backlogs",
			"New workers can drain existing backlogs",
		},
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

	if err := h.publishAndBroadcast("game.quest.gather", testPayload, "chaos-test-scenario"); err != nil {
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
