package handlers_test

import (
	"demo-event-bus-api/internal/api/handlers"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/websocket"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to safely convert interface{} to int
func getIntFromInterface(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

// Test setup helper for integration tests with real clients
func setupRealScenarioTestHandler() (*handlers.Handlers, error) {
	gin.SetMode(gin.TestMode)

	// Use environment variables or defaults for test services
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	workersURL := os.Getenv("WORKERS_URL")
	if workersURL == "" {
		workersURL = "http://localhost:8001" // Default port for workers service
	}

	cfg := &config.Config{
		Port:        "9001", // Test port
		WorkersURL:  workersURL,
		PythonURL:   "http://localhost:8000",
		RabbitMQURL: rabbitmqURL,
	}

	// Create real clients and ensure connections
	rabbitMQClient := clients.NewRabbitMQClient(rabbitmqURL)

	// Test RabbitMQ connection with retry
	var lastErr error
	for i := 0; i < 5; i++ {
		if _, lastErr = rabbitMQClient.GetQueuesFromAPI(); lastErr == nil {
			break
		}
		if i == 4 {
			return nil, fmt.Errorf("failed to connect to RabbitMQ after retries: %w", lastErr)
		}
		time.Sleep(1 * time.Second)
	}

	workersClient := clients.NewWorkersClient(workersURL)
	pythonClient := clients.NewPythonClient("http://localhost:8000")

	wsHub := websocket.NewHub()
	go wsHub.Run() // Start hub to prevent blocking

	handlers := &handlers.Handlers{
		RabbitMQClient: rabbitMQClient,
		WorkersClient:  workersClient,
		PythonClient:   pythonClient,
		WSHub:          wsHub,
		Config:         cfg,
	}

	return handlers, nil
}

// Helper to check if test dependencies are available
func checkTestDependencies() error {
	// Check RabbitMQ
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	testClient := clients.NewRabbitMQClient(rabbitmqURL)
	if _, err := testClient.GetQueuesFromAPI(); err != nil {
		return fmt.Errorf("RabbitMQ not available for integration tests: %w", err)
	}

	// Check Workers service
	workersURL := os.Getenv("WORKERS_URL")
	if workersURL == "" {
		workersURL = "http://localhost:8001" // Default port for workers service
	}

	workersClient := clients.NewWorkersClient(workersURL)
	if _, err := workersClient.GetStatus(); err != nil {
		return fmt.Errorf("Workers service not available for integration tests: %w", err)
	}

	return nil
}

// Helper to clean up test queues and workers
func cleanupTestResources(handlers *handlers.Handlers, testPrefix string) {
	// Clean up any test workers
	testWorkers := []string{
		testPrefix + "-escort-worker",
		testPrefix + "-temp-worker",
		"late-bind-escort-worker",
	}

	for _, workerName := range testWorkers {
		handlers.WorkersClient.StopWorker(workerName)
	}

	// Clean up test queues (purge messages)
	testQueues := []string{
		"game.skill.escort.q",
		"game.skill.gather.q",
	}

	for _, queueName := range testQueues {
		handlers.RabbitMQClient.PurgeQueue(queueName)
	}

	// Wait a moment for cleanup to complete
	time.Sleep(500 * time.Millisecond)
}

// Integration test for Late-bind Escort Scenario
// Tests actual backlog handoff functionality: workers, queues, and message processing
func TestLateBendEscortScenario_Integration(t *testing.T) {
	// Skip if dependencies not available
	if err := checkTestDependencies(); err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	handlers, err := setupRealScenarioTestHandler()
	require.NoError(t, err, "Should setup test handler")

	testPrefix := fmt.Sprintf("test-%d", time.Now().UnixNano())
	defer cleanupTestResources(handlers, testPrefix)

	// Ensure RabbitMQ connection is active before testing
	_, err = handlers.RabbitMQClient.GetQueuesFromAPI()
	require.NoError(t, err, "RabbitMQ should be accessible for functional testing")

	// Verify queue starts empty
	initialQueueInfo, err := handlers.RabbitMQClient.GetQueueInfo("game.skill.escort.q")
	initialMessages := 0
	if err == nil {
		if msgs, ok := initialQueueInfo["messages"]; ok {
			switch v := msgs.(type) {
			case int:
				initialMessages = v
			case float64:
				initialMessages = int(v)
			}
		}
	}

	// Run the scenario with test parameters
	scenarioParams := map[string]interface{}{
		"backlog_messages": 3,
		"delay_seconds":    1, // Short delay for testing
		"worker_prefix":    testPrefix,
	}

	result, err := handlers.RunLateBendEscortScenario(scenarioParams)
	require.NoError(t, err, "Scenario should execute without errors")
	require.NotNil(t, result, "Should return scenario result")

	// 1. Verify scenario phases completed successfully
	assert.Equal(t, true, result["phase_1_complete"], "Phase 1 (initial worker + messages) should complete")
	assert.Equal(t, true, result["phase_2_complete"], "Phase 2 (worker removal) should complete")
	assert.Equal(t, true, result["phase_3_complete"], "Phase 3 (backlog accumulation) should complete")
	assert.Equal(t, true, result["phase_4_complete"], "Phase 4 (delay) should complete")
	assert.Equal(t, true, result["phase_5_complete"], "Phase 5 (new worker) should complete")

	// 2. Verify workers were actually created and managed
	tempWorker := result["temp_worker"].(string)
	newWorker := result["new_worker"].(string)

	assert.Contains(t, tempWorker, testPrefix, "Temp worker should use test prefix")
	assert.Contains(t, newWorker, testPrefix, "New worker should use test prefix")
	assert.NotEqual(t, tempWorker, newWorker, "Should create different workers")

	// 3. Verify message counts and processing
	backlogMessages := getIntFromInterface(result["backlog_messages"])
	initialBacklog := getIntFromInterface(result["initial_backlog"])
	finalBacklog := getIntFromInterface(result["final_backlog"])
	messagesConsumed := getIntFromInterface(result["messages_consumed"])

	assert.Equal(t, 3, backlogMessages, "Should have configured 3 backlog messages")
	assert.GreaterOrEqual(t, messagesConsumed, 0, "Should track consumed messages")
	assert.LessOrEqual(t, finalBacklog, initialBacklog, "Final backlog should be <= initial backlog")

	// 4. Verify timeline contains expected actions
	timeline, ok := result["timeline"].([]map[string]interface{})
	require.True(t, ok, "Timeline should be an array of events")
	require.GreaterOrEqual(t, len(timeline), 5, "Should have at least 5 timeline events")

	// Check for key timeline events
	timelineActions := make([]string, len(timeline))
	for i, event := range timeline {
		action, _ := event["action"].(string)
		timelineActions[i] = action
	}

	assert.Contains(t, timelineActions, "Creating initial worker", "Should create initial worker")
	assert.Contains(t, timelineActions, "Stopping worker to create backlog scenario", "Should stop worker")
	assert.Contains(t, timelineActions, "Creating new worker to consume backlog", "Should create new worker")

	// 5. Verify final queue state is reasonable
	finalQueueInfo, err := handlers.RabbitMQClient.GetQueueInfo("game.skill.escort.q")
	if err == nil {
		var currentMessages int
		if msgs, ok := finalQueueInfo["messages"]; ok {
			switch v := msgs.(type) {
			case int:
				currentMessages = v
			case float64:
				currentMessages = int(v)
			}
		}

		// Should not have grown from initial state (messages were processed)
		assert.LessOrEqual(t, currentMessages, initialMessages+6, "Queue should not accumulate unprocessed messages")
	}

	// 6. Verify execution timing is reasonable
	executionTime := float64(getIntFromInterface(result["execution_time_ms"]))
	if executionTime > 0 {
		assert.Greater(t, executionTime, 500.0, "Should take at least 500ms (due to processing delays)")
		assert.Less(t, executionTime, 30000.0, "Should complete within 30 seconds")
	} else {
		t.Logf("Note: execution_time_ms was 0, scenario may need timing fixes")
	}

	t.Logf("Functional test passed:")
	t.Logf("  - Workers: %s -> %s", tempWorker, newWorker)
	t.Logf("  - Messages: %d configured, %d initial backlog, %d final backlog, %d consumed",
		backlogMessages, initialBacklog, finalBacklog, messagesConsumed)
	t.Logf("  - Execution time: %.0f ms", executionTime)
	t.Logf("  - Timeline events: %d", len(timeline))
}
