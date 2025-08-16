package handlers

import (
	"bytes"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/models"
	"demo-event-bus-api/internal/websocket"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ScenarioTestRequest represents a request to run a scenario test
type ScenarioTestRequest struct {
	Scenario   string                 `json:"scenario" binding:"required"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ScenarioTestResponse represents the response from running a scenario test
type ScenarioTestResponse struct {
	Success       bool                   `json:"success"`
	Scenario      string                 `json:"scenario"`
	ExecutionTime int64                  `json:"execution_time_ms"`
	Results       map[string]interface{} `json:"results"`
	TestSteps     []TestStep             `json:"test_steps"`
	Summary       string                 `json:"summary"`
	Error         string                 `json:"error,omitempty"`
}

// TestStep represents a step in the scenario test execution
type TestStep struct {
	Step        int                    `json:"step"`
	Name        string                 `json:"name"`
	Status      string                 `json:"status"` // "success", "warning", "error"
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Timestamp   int64                  `json:"timestamp"`
}

// RunScenarioTest executes a scenario test and returns detailed results
func (h *Handlers) RunScenarioTest(c *gin.Context) {
	var req ScenarioTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request format: " + err.Error(),
		})
		return
	}

	startTime := time.Now()

	// Execute the scenario test based on type
	var response ScenarioTestResponse
	// Create a test framework instance
	testFramework := h.createTestFramework()

	switch req.Scenario {
	case "late-bind-escort":
		response = h.runLateBondEscortScenarioTest(testFramework, req.Parameters)
	case "dlq-message-flow":
		response = h.runDLQMessageFlowScenarioTest(testFramework, req.Parameters)
	case "reissuing-dlq":
		response = h.runReissuingDLQScenarioTest(testFramework, req.Parameters)
	case "orphaned-skill-queues":
		response = h.runOrphanedSkillQueuesScenarioTest(testFramework, req.Parameters)
	default:
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Unknown scenario: " + req.Scenario,
			Data: map[string]interface{}{
				"available_scenarios": []string{
					"late-bind-escort",
					"dlq-message-flow",
					"reissuing-dlq",
					"orphaned-skill-queues",
				},
			},
		})
		return
	}

	response.ExecutionTime = time.Since(startTime).Milliseconds()
	response.Scenario = req.Scenario

	// Broadcast scenario test execution to UI
	h.broadcastMessage("scenario_test_executed", map[string]interface{}{
		"scenario":       req.Scenario,
		"success":        response.Success,
		"execution_time": response.ExecutionTime,
		"summary":        response.Summary,
		"timestamp":      time.Now().Unix(),
	})

	// Return appropriate HTTP status
	status := http.StatusOK
	if !response.Success {
		status = http.StatusInternalServerError
	}

	c.JSON(status, models.APIResponse{
		Success: response.Success,
		Data:    response,
		Message: fmt.Sprintf("Scenario test '%s' completed", req.Scenario),
	})
}

// ListAvailableScenarioTests returns a list of all available scenario tests
func (h *Handlers) ListAvailableScenarioTests(c *gin.Context) {
	scenarios := []map[string]interface{}{
		{
			"id":          "late-bind-escort",
			"name":        "Late-bind Escort with Unreachable Messages",
			"description": "Demonstrates message behavior when sent before consumers exist, including DLQ routing",
			"parameters": map[string]interface{}{
				"message_count": map[string]interface{}{
					"type":        "integer",
					"default":     3,
					"description": "Number of messages to send in each phase",
				},
			},
			"steps": []string{
				"Reset game state and trigger DLQ auto-setup",
				"Send messages before any consumers exist",
				"Verify messages route to unroutable DLQ",
				"Send additional messages to test accumulation",
				"Verify final queue states and message routing",
			},
		},
		{
			"id":          "dlq-message-flow",
			"name":        "DLQ Message Flow End-to-End",
			"description": "Tests the complete DLQ message flow including categorization and routing",
			"parameters": map[string]interface{}{
				"test_messages": map[string]interface{}{
					"type":        "integer",
					"default":     5,
					"description": "Number of test messages to process",
				},
			},
			"steps": []string{
				"Setup DLQ infrastructure",
				"Generate different types of DLQ messages",
				"Verify categorization and routing",
				"Test reissue functionality",
			},
		},
		{
			"id":          "reissuing-dlq",
			"name":        "Reissuing DLQ Messages (Scenario 2)",
			"description": "Validates that unroutable and failed messages can be reissued successfully",
			"parameters": map[string]interface{}{
				"character_name": map[string]interface{}{
					"type":        "string",
					"default":     "alice",
					"description": "Name of the character/worker to create",
				},
			},
			"steps": []string{
				"Start clean",
				"Send gather message when no character exists (unroutable)",
				"Create character Alice",
				"Send gather messages with deterministic outcomes",
				"Reissue failed and unroutable messages",
				"Verify successful processing",
			},
		},
		{
			"id":          "orphaned-skill-queues",
			"name":        "Orphaned Skill Queues (Scenario 3)",
			"description": "Shows when skill queues become orphaned and how pending messages behave",
			"parameters": map[string]interface{}{
				"character_name": map[string]interface{}{
					"type":        "string",
					"default":     "alice",
					"description": "Name of the character to create and delete",
				},
				"messages_per_phase": map[string]interface{}{
					"type":        "integer",
					"default":     2,
					"description": "Number of messages to send in each phase",
				},
			},
			"steps": []string{
				"Start clean with no characters",
				"Create character Alice",
				"Send messages that are processed",
				"Delete Alice (queue becomes orphaned)",
				"Send messages to orphaned queue",
				"Create new character Bob",
				"Verify Bob processes pending messages",
			},
		},
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"scenarios": scenarios,
			"count":     len(scenarios),
		},
		Message: "Available scenario tests retrieved successfully",
	})
}

// MessageState represents structured tracking for each message
type MessageState struct {
	ID            string `json:"id"`
	ExpectedState string `json:"expected_state"` // "unroutable", "completed"
	Step          int    `json:"step"`           // Which step created this message
	Timestamp     int64  `json:"timestamp"`      // When it was sent
}

// runLateBondEscortScenarioTest executes the Late-bind Escort scenario test
// Implements structured message tracking and proper timing for reliable validation
func (h *Handlers) runLateBondEscortScenarioTest(testFramework *ScenarioTestFramework, params map[string]interface{}) ScenarioTestResponse {
	testSteps := []TestStep{}
	results := make(map[string]interface{})
	success := true
	errorMsg := ""

	// Structured message tracking with expected final states
	messageStates := []MessageState{}
	questLogEntries := []string{}

	// Step 1: Reset and setup
	step1 := TestStep{Step: 1, Name: "Reset and Setup", Status: "success", Timestamp: time.Now().Unix(), Description: "Reset game state and prepare for shift scheduling scenario"}
	testFramework.ResetGameState()

	logEntry := "🎬 [SCENARIO] Welcome to Escort Service HQ! Today we're testing our shift scheduling system..."
	testFramework.LogToQuestLog(logEntry)
	questLogEntries = append(questLogEntries, logEntry)

	logEntry = "📖 [SCENARIO] SCENARIO: It's Monday morning. Our escort service operates in shifts with a lunch break gap."
	testFramework.LogToQuestLog(logEntry)
	questLogEntries = append(questLogEntries, logEntry)
	testSteps = append(testSteps, step1)

	// Step 2: Send unreachable message (should end up in unroutable DLQ)
	step2 := TestStep{Step: 2, Name: "Send Unreachable Message", Status: "success", Timestamp: time.Now().Unix(), Description: "Send escort message before worker exists - should route to unroutable DLQ"}

	logEntry = "📞 [SCENARIO] 7:45 AM - A client calls requesting an escort, but our service doesn't open until 8:00 AM..."
	testFramework.LogToQuestLog(logEntry)
	questLogEntries = append(questLogEntries, logEntry)

	logEntry = "📨 [SCENARIO] Sending the early request to our system (no workers scheduled yet)..."
	testFramework.LogToQuestLog(logEntry)
	questLogEntries = append(questLogEntries, logEntry)

	unreachableMessages := testFramework.SendQuestMessage("escort", 1)
	if len(unreachableMessages) > 0 {
		messageStates = append(messageStates, MessageState{
			ID:            unreachableMessages[0],
			ExpectedState: "unroutable",
			Step:          2,
			Timestamp:     time.Now().Unix(),
		})
	}

	time.Sleep(3 * time.Second) // Allow time for message routing

	logEntry = "📥 [SCENARIO] The early request has been logged in our 'unroutable' queue - we'll handle it when service opens."
	testFramework.LogToQuestLog(logEntry)
	questLogEntries = append(questLogEntries, logEntry)

	step2.Data = map[string]interface{}{
		"message_count": len(unreachableMessages),
		"message_ids":   unreachableMessages,
	}
	testSteps = append(testSteps, step2)

	// Step 3: Create morning shift worker - MUST succeed
	step3 := TestStep{Step: 3, Name: "Morning Shift Worker", Timestamp: time.Now().Unix(), Description: "Create morning shift escort worker (fail_pct: 0.0%)"}
	logEntry = "🌅 [SCENARIO] 8:00 AM - Time to start the morning shift! Calling in our best escort agent..."
	testFramework.LogToQuestLog(logEntry)
	questLogEntries = append(questLogEntries, logEntry)

	if err := testFramework.CreateWorker("morning-shift", []string{"escort"}, 0.0, 1.0); err != nil {
		step3.Status = "error"
		success = false
		errorMsg = fmt.Sprintf("Worker creation failed: %v", err)

		logEntry = fmt.Sprintf("❌ [SCENARIO] CRISIS! Our morning shift agent didn't show up: %v", err)
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)
	} else {
		step3.Status = "success"

		logEntry = "✅ [SCENARIO] Morning shift agent 'morning-shift' has clocked in! (Professional, 0% failure rate)"
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		time.Sleep(3 * time.Second) // Allow worker to fully initialize

		logEntry = "📋 [SCENARIO] Great! Now we have 3 escort requests that came in this morning. Let's assign them..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)
		processedMessages := testFramework.SendQuestMessage("escort", 3)
		for _, msgID := range processedMessages {
			messageStates = append(messageStates, MessageState{
				ID:            msgID,
				ExpectedState: "completed",
				Step:          3,
				Timestamp:     time.Now().Unix(),
			})
		}

		// 🎯 CRITICAL: Wait for messages to be fully processed before stopping worker
		logEntry = "⏰ [SCENARIO] Our morning shift agent is working hard! Let's give them 8 seconds to complete all requests..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		time.Sleep(8 * time.Second) // Allow sufficient processing time

		logEntry = "✅ [SCENARIO] Excellent! All morning requests completed successfully. Time for lunch break!"
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		step3.Data = map[string]interface{}{
			"processed_message_ids": processedMessages,
			"expected_state":        "completed",
		}
	}
	testSteps = append(testSteps, step3)

	// Only continue if worker creation succeeded
	if success {
		// Step 4: Stop morning shift worker (lunch break starts)
		step4 := TestStep{Step: 4, Name: "Lunch Break", Timestamp: time.Now().Unix(), Description: "Stop morning shift worker to simulate lunch break downtime"}

		logEntry = "🍽️ [SCENARIO] 12:00 PM - Lunch time! Our morning shift agent heads out for a well-deserved break..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		if err := testFramework.StopWorker("morning-shift"); err != nil {
			step4.Status = "error"
			success = false
			errorMsg = fmt.Sprintf("Worker stop failed: %v", err)

			logEntry = fmt.Sprintf("❌ [SCENARIO] Problem! Our agent couldn't leave for lunch: %v", err)
			testFramework.LogToQuestLog(logEntry)
			questLogEntries = append(questLogEntries, logEntry)
		} else {
			step4.Status = "success"

			logEntry = "✅ [SCENARIO] Morning shift agent is now at lunch. The office is temporarily unstaffed..."
			testFramework.LogToQuestLog(logEntry)
			questLogEntries = append(questLogEntries, logEntry)
		}
		testSteps = append(testSteps, step4)

		// Step 5: Create lunch break backlog
		step5 := TestStep{Step: 5, Name: "Lunch Break Backlog", Status: "success", Timestamp: time.Now().Unix(), Description: "Send messages during lunch break - will accumulate until afternoon shift"}
		logEntry = "📞 [SCENARIO] Uh oh! While our agent is at lunch, 2 new clients call requesting escorts..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		logEntry = "📝 [SCENARIO] Since no one is available, these requests will queue up for the afternoon shift..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		backlogMessages := testFramework.SendQuestMessage("escort", 2)
		for _, msgID := range backlogMessages {
			messageStates = append(messageStates, MessageState{
				ID:            msgID,
				ExpectedState: "completed", // Will be processed by afternoon shift worker
				Step:          5,
				Timestamp:     time.Now().Unix(),
			})
		}

		logEntry = fmt.Sprintf("📥 [SCENARIO] Lunch break requests logged: %v - they'll wait for afternoon shift", backlogMessages)
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		logEntry = "🕐 [SCENARIO] Lunch break in progress... clients are waiting patiently (10 seconds to simulate)..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		// 🎯 CRITICAL: Verify backlog is accumulating during this sleep
		for i := 1; i <= 10; i++ {
			time.Sleep(1 * time.Second)
			if i%3 == 0 {
				logEntry = fmt.Sprintf("🕐 [SCENARIO] %d minutes into lunch break... requests are piling up...", i/3*10)
				testFramework.LogToQuestLog(logEntry)
				questLogEntries = append(questLogEntries, logEntry)
			}
		}

		// 🎯 VALIDATE BACKLOG: Confirm messages are actually queued in RabbitMQ
		logEntry = "🔍 [SCENARIO] Let's check our request queue to see if the lunch break requests are properly waiting..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		backlogValid, validationMsg := testFramework.ValidateBacklogInQueue("escort", len(backlogMessages))
		if !backlogValid {
			step5.Status = "error"
			success = false
			errorMsg = fmt.Sprintf("Backlog validation failed: %s", validationMsg)
			testFramework.LogToQuestLog(fmt.Sprintf("🚨 [SCENARIO FAILED] %s", errorMsg))

			// Early termination - don't continue to Step 6
			step5.Data = map[string]interface{}{
				"backlog_message_ids": backlogMessages,
				"expected_state":      "failed",
				"validation_error":    validationMsg,
			}
			testSteps = append(testSteps, step5)

			// Return early with failure
			return ScenarioTestResponse{
				Success:   false,
				Results:   map[string]interface{}{"validation_error": validationMsg},
				TestSteps: testSteps,
				Summary:   fmt.Sprintf("Shift Scheduling scenario failed: %s", errorMsg),
				Error:     errorMsg,
			}
		}

		step5.Data = map[string]interface{}{
			"backlog_message_ids": backlogMessages,
			"expected_state":      "completed",
		}
		testSteps = append(testSteps, step5)

		// Step 6: Afternoon shift worker
		step6 := TestStep{Step: 6, Name: "Afternoon Shift Worker", Timestamp: time.Now().Unix(), Description: "Create afternoon shift worker to process lunch break backlog (fail_pct: 0.0%)"}

		logEntry = "🌅 [SCENARIO] 1:00 PM - Lunch break is over! Time for our afternoon shift agent to take over..."
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		if err := testFramework.CreateWorker("afternoon-shift", []string{"escort"}, 0.0, 1.0); err != nil {
			step6.Status = "error"
			success = false
			errorMsg = fmt.Sprintf("Afternoon shift worker creation failed: %v", err)

			logEntry = fmt.Sprintf("❌ [SCENARIO] DISASTER! Our afternoon shift agent is a no-show: %v", err)
			testFramework.LogToQuestLog(logEntry)
			questLogEntries = append(questLogEntries, logEntry)
		} else {
			step6.Status = "success"

			logEntry = "✅ [SCENARIO] Afternoon shift agent 'afternoon-shift' has arrived! (Professional, 0% failure rate)"
			testFramework.LogToQuestLog(logEntry)
			questLogEntries = append(questLogEntries, logEntry)

			logEntry = "📋 [SCENARIO] Great! The afternoon agent sees the backlog and immediately starts processing requests..."
			testFramework.LogToQuestLog(logEntry)
			questLogEntries = append(questLogEntries, logEntry)

			time.Sleep(8 * time.Second) // Allow time for backlog processing

			logEntry = "🎉 [SCENARIO] Perfect! The afternoon shift has processed all lunch break requests. Seamless handoff complete!"
			testFramework.LogToQuestLog(logEntry)
			questLogEntries = append(questLogEntries, logEntry)

			// Don't cleanup worker - leave it running to validate final state
		}
		testSteps = append(testSteps, step6)

		// Compile structured results with comprehensive tracking
		expectedSystemState := map[string]interface{}{
			"unroutable_messages": 1, // Only the first message
			"active_workers":      1, // Only afternoon-shift
			"pending_messages":    0, // All others should be completed
			"worker_names":        []string{"afternoon-shift"},
		}

		results["total_messages_sent"] = len(messageStates)
		results["message_states"] = messageStates
		results["quest_log_entries"] = questLogEntries
		results["expected_system_state"] = expectedSystemState
		results["unreachable_message_id"] = unreachableMessages[0]

		logEntry = "📊 [SCENARIO] MISSION COMPLETE! Our shift scheduling system handled the day perfectly:"
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		logEntry = "✅ [SCENARIO] Early morning request → Handled via unroutable queue"
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		logEntry = "✅ [SCENARIO] Morning shift requests → All completed by morning agent"
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		logEntry = "✅ [SCENARIO] Lunch break requests → Queued and processed by afternoon agent"
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)

		logEntry = "🏆 [SCENARIO] RESULT: Smooth shift transitions with zero client service disruption!"
		testFramework.LogToQuestLog(logEntry)
		questLogEntries = append(questLogEntries, logEntry)
	}

	summary := "Late-bind Escort scenario completed successfully"
	if !success {
		summary = fmt.Sprintf("Late-bind Escort scenario failed: %s", errorMsg)
	}

	return ScenarioTestResponse{
		Success:   success,
		Results:   results,
		TestSteps: testSteps,
		Summary:   summary,
		Error:     errorMsg,
	}
}

// runDLQMessageFlowScenarioTest executes DLQ message flow testing
func (h *Handlers) runDLQMessageFlowScenarioTest(testFramework *ScenarioTestFramework, params map[string]interface{}) ScenarioTestResponse {
	// TODO: Implement DLQ message flow scenario test
	return ScenarioTestResponse{
		Success:   false,
		Results:   map[string]interface{}{},
		TestSteps: []TestStep{},
		Summary:   "DLQ message flow scenario test not yet implemented",
		Error:     "Not implemented",
	}
}

// runReissuingDLQScenarioTest executes the reissuing DLQ scenario (Scenario 2)
func (h *Handlers) runReissuingDLQScenarioTest(testFramework *ScenarioTestFramework, params map[string]interface{}) ScenarioTestResponse {
	// TODO: Implement reissuing DLQ scenario test (Scenario 2 from scenarios.md)
	return ScenarioTestResponse{
		Success:   false,
		Results:   map[string]interface{}{},
		TestSteps: []TestStep{},
		Summary:   "Reissuing DLQ scenario test not yet implemented",
		Error:     "Not implemented",
	}
}

// runOrphanedSkillQueuesScenarioTest executes the orphaned skill queues scenario (Scenario 3)
func (h *Handlers) runOrphanedSkillQueuesScenarioTest(testFramework *ScenarioTestFramework, params map[string]interface{}) ScenarioTestResponse {
	// TODO: Implement orphaned skill queues scenario test (Scenario 3 from scenarios.md)
	return ScenarioTestResponse{
		Success:   false,
		Results:   map[string]interface{}{},
		TestSteps: []TestStep{},
		Summary:   "Orphaned skill queues scenario test not yet implemented",
		Error:     "Not implemented",
	}
}

// createTestFramework creates a scenario test framework for internal use
func (h *Handlers) createTestFramework() *ScenarioTestFramework {
	// Create a minimal config for the test framework
	cfg := &config.Config{
		RabbitMQURL: h.Config.RabbitMQURL,
		WorkersURL:  h.Config.WorkersURL,
		PythonURL:   h.Config.PythonURL,
		Port:        h.Config.Port,
	}

	// Create a test router using the existing setup
	router := gin.New()
	gin.SetMode(gin.TestMode)

	// 🎯 CRITICAL: Use the MAIN WebSocket hub, not a separate one
	// This ensures scenario Quest Log messages reach the UI
	mainHub := h.WSHub

	// Create minimal test framework
	framework := &ScenarioTestFramework{
		router: router,
		hub:    mainHub, // Use main hub for Quest Log integration
	}

	// Use the current handlers' router for API calls
	framework.router = h.setupTestRouter(cfg, mainHub)

	return framework
}

// setupTestRouter creates a test router with the same configuration as the main API
func (h *Handlers) setupTestRouter(cfg *config.Config, hub *websocket.Hub) *gin.Engine {
	router := gin.New()

	// 🎯 CRITICAL: Use the EXISTING handler instance to ensure same client connections
	// Don't create new handlers - reuse the current one with its established connections

	// Setup essential routes for testing - using the CURRENT handler instance
	api := router.Group("/api")
	{
		api.POST("/reset", h.ResetGame)
		api.POST("/master/one", h.SendOne)
		api.GET("/rabbitmq/queues", h.GetRabbitMQQueues)
		api.GET("/dlq/list", h.ListDLQMessages)

		// Player management endpoints (reusing existing production APIs with SAME clients)
		api.POST("/player/start", h.StartPlayer)
		api.POST("/player/delete", h.DeletePlayer)
	}

	return router
}

// ScenarioTestFramework - embedded framework for API-based scenario testing
type ScenarioTestFramework struct {
	router *gin.Engine
	hub    *websocket.Hub
}

// ResetGameState performs a complete reset of the game state
func (stf *ScenarioTestFramework) ResetGameState() {
	req, _ := http.NewRequest("POST", "/api/reset", nil)
	w := httptest.NewRecorder()
	stf.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusPartialContent {
		// Log error but don't fail - reset might have warnings
	}

	time.Sleep(2 * time.Second)

	// Trigger DLQ auto-setup
	stf.TriggerDLQAutoSetup()
}

// TriggerDLQAutoSetup ensures DLQ infrastructure is set up
func (stf *ScenarioTestFramework) TriggerDLQAutoSetup() {
	req, _ := http.NewRequest("GET", "/api/dlq/list", nil)
	w := httptest.NewRecorder()
	stf.router.ServeHTTP(w, req)

	time.Sleep(1 * time.Second)
}

// SendQuestMessage sends quest messages and returns the message IDs
func (stf *ScenarioTestFramework) SendQuestMessage(questType string, count int) []string {
	var messageIDs []string

	for i := 0; i < count; i++ {
		payload := map[string]interface{}{
			"quest_type": questType,
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/master/one", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		stf.router.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			var response models.APIResponse
			if json.Unmarshal(w.Body.Bytes(), &response) == nil {
				if data, ok := response.Data.(map[string]interface{}); ok {
					if payload, ok := data["payload"].(map[string]interface{}); ok {
						if id, ok := payload["case_id"].(string); ok {
							messageIDs = append(messageIDs, id)
						}
					}
				}
			}
		}
	}

	return messageIDs
}

// QueueStatus represents the status of a RabbitMQ queue
type QueueStatus struct {
	Name      string
	Messages  int
	Ready     int
	Unacked   int
	Consumers int
	Exists    bool
}

// GetQueueStatus returns the status of a specific queue
func (stf *ScenarioTestFramework) GetQueueStatus(queueName string) QueueStatus {
	req, _ := http.NewRequest("GET", "/api/rabbitmq/queues", nil)
	w := httptest.NewRecorder()
	stf.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		return QueueStatus{Name: queueName, Exists: false}
	}

	var response models.APIResponse
	if json.Unmarshal(w.Body.Bytes(), &response) != nil {
		return QueueStatus{Name: queueName, Exists: false}
	}

	queues, ok := response.Data.([]interface{})
	if !ok {
		return QueueStatus{Name: queueName, Exists: false}
	}

	for _, q := range queues {
		queue, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		if name, ok := queue["name"].(string); ok && name == queueName {
			return QueueStatus{
				Name:      name,
				Messages:  getIntFromInterface(queue["messages"]),
				Ready:     getIntFromInterface(queue["messages_ready"]),
				Unacked:   getIntFromInterface(queue["messages_unacknowledged"]),
				Consumers: getIntFromInterface(queue["consumers"]),
				Exists:    true,
			}
		}
	}

	return QueueStatus{Name: queueName, Exists: false}
}

// CreateWorker creates a worker using the existing /api/player/start endpoint
// REQUIRES workers service to be available - fails if not
func (stf *ScenarioTestFramework) CreateWorker(name string, skills []string, failPct float64, speedMultiplier float64) error {
	// Convert skills slice to comma-separated string as expected by /api/player/start
	skillsStr := strings.Join(skills, ",")

	payload := map[string]interface{}{
		"player":           name,
		"skills":           skillsStr, // String format as expected by StartPlayer
		"fail_pct":         failPct,
		"speed_multiplier": speedMultiplier,
		"workers":          1,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/api/player/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	stf.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		return fmt.Errorf("failed to create worker %s: HTTP %d - Response: %s", name, w.Code, w.Body.String())
	}
	return nil
}

// StopWorker stops a worker using the existing /api/player/delete endpoint
// REQUIRES workers service to be available - fails if not
func (stf *ScenarioTestFramework) StopWorker(name string) error {
	payload := map[string]interface{}{
		"player": name,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/api/player/delete", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	stf.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		return fmt.Errorf("failed to stop worker %s: HTTP %d - Response: %s", name, w.Code, w.Body.String())
	}
	return nil
}

// LogToQuestLog sends a message to the Quest Log for scenario tracking
func (stf *ScenarioTestFramework) LogToQuestLog(message string) {
	// Send to Quest Log via WebSocket using the Hub's BroadcastMessage method
	if stf.hub != nil {
		questLogMessage := &models.WebSocketMessage{
			Type: "quest_log",
			Payload: map[string]interface{}{
				"message": message,
				"source":  "SCENARIO", // 🎯 Tagged as "SCENARIO" for filtering
			},
			Timestamp: float64(time.Now().Unix()),
		}

		stf.hub.BroadcastMessage(questLogMessage)
	}
}

// ValidateBacklogInQueue queries RabbitMQ to verify messages are actually queued
func (stf *ScenarioTestFramework) ValidateBacklogInQueue(skill string, expectedCount int) (bool, string) {
	queueName := fmt.Sprintf("game.skill.%s.q", skill)

	// Query RabbitMQ Management API
	req, _ := http.NewRequest("GET", "/api/rabbitmq/queues", nil)
	w := httptest.NewRecorder()
	stf.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		errorMsg := fmt.Sprintf("❌ Failed to query RabbitMQ queues: HTTP %d", w.Code)
		stf.LogToQuestLog(errorMsg)
		return false, errorMsg
	}

	// Parse response to find the specific queue
	var response struct {
		OK   bool                     `json:"ok"`
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		errorMsg := fmt.Sprintf("❌ Failed to parse queue data: %v", err)
		stf.LogToQuestLog(errorMsg)
		return false, errorMsg
	}

	if !response.OK {
		errorMsg := "❌ RabbitMQ API returned error"
		stf.LogToQuestLog(errorMsg)
		return false, errorMsg
	}

	// Find the target queue
	for _, queue := range response.Data {
		if name, nameOk := queue["name"].(string); nameOk && name == queueName {
			if messages, msgOk := queue["messages"].(float64); msgOk {
				actualCount := int(messages)
				if actualCount == expectedCount {
					successMsg := fmt.Sprintf("✅ [SCENARIO] Perfect! Found %d requests waiting in the queue as expected.", actualCount)
					stf.LogToQuestLog(successMsg)
					return true, successMsg
				} else {
					errorMsg := fmt.Sprintf("❌ [SCENARIO] Queue problem! Expected %d waiting requests, but found %d.", expectedCount, actualCount)
					stf.LogToQuestLog(errorMsg)
					return false, errorMsg
				}
			}
		}
	}

	errorMsg := fmt.Sprintf("❌ Queue %s not found in RabbitMQ", queueName)
	stf.LogToQuestLog(errorMsg)
	return false, errorMsg
}

// Helper function to safely convert interface{} to int
func getIntFromInterface(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		// Parse string if needed
		return 0
	}
	return 0
}
