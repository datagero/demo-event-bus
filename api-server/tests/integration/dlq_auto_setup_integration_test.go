package handlers_test

import (
	"bytes"
	"demo-event-bus-api/internal/api/handlers"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/models"
	"demo-event-bus-api/internal/websocket"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDLQAutoSetupFunctionality tests the auto-setup feature for DLQ
func TestDLQAutoSetupFunctionality(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
		Port:        "9001", // Test port
		WorkersURL:  "http://localhost:8001",
		PythonURL:   "http://localhost:8000",
	}

	// Initialize clients
	rabbitMQClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)
	// Test RabbitMQ connection
	if _, err := rabbitMQClient.GetQueuesFromAPI(); err != nil {
		t.Skip("RabbitMQ not available for integration test")
	}

	workersClient := clients.NewWorkersClient(cfg.WorkersURL)
	pythonClient := clients.NewPythonClient(cfg.PythonURL)
	hub := websocket.NewHub()
	go hub.Run() // Start hub to prevent blocking

	// Create handlers
	h := &handlers.Handlers{
		RabbitMQClient: rabbitMQClient,
		WorkersClient:  workersClient,
		PythonClient:   pythonClient,
		WSHub:          hub,
		Config:         cfg,
	}

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add DLQ routes
	dlq := router.Group("/api/dlq")
	{
		dlq.GET("/list", h.ListDLQMessages)
		dlq.POST("/setup", h.SetupDLQ)
	}

	// Add reset route
	router.POST("/api/reset", h.ResetGame)

	t.Run("Auto_Setup_Triggers_When_No_DLQ_Exists", func(t *testing.T) {
		// 1. Ensure clean state by performing reset
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Wait for reset to complete
		time.Sleep(2 * time.Second)

		// 2. Verify no DLQ queues exist in RabbitMQ
		queues, err := rabbitMQClient.GetQueuesFromAPI()
		require.NoError(t, err)

		dlqCount := 0
		for _, queue := range queues {
			if queueName, ok := queue["name"].(string); ok {
				if h.IsDLQQueue(queueName) {
					dlqCount++
				}
			}
		}
		assert.Equal(t, 0, dlqCount, "Should have no DLQ queues after reset")

		// 3. Call DLQ list endpoint (should trigger auto-setup)
		req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)

		// 4. Verify auto-setup created DLQ queues
		if data, ok := response.Data.(map[string]interface{}); ok {
			if dlqQueues, exists := data["dlq_queues"]; exists {
				if queuesSlice, ok := dlqQueues.([]interface{}); ok {
					assert.GreaterOrEqual(t, len(queuesSlice), 4, "Should have created at least 4 DLQ queues")
				}
			}
		}

		// 5. Verify DLQ queues exist in RabbitMQ
		queues, err = rabbitMQClient.GetQueuesFromAPI()
		require.NoError(t, err)

		dlqQueueNames := []string{}
		for _, queue := range queues {
			if queueName, ok := queue["name"].(string); ok {
				if h.IsDLQQueue(queueName) {
					dlqQueueNames = append(dlqQueueNames, queueName)
				}
			}
		}

		expectedQueues := []string{
			"game.dlq.failed.q",
			"game.dlq.unroutable.q",
			"game.dlq.expired.q",
			"game.dlq.retry.q",
		}

		for _, expectedQueue := range expectedQueues {
			assert.Contains(t, dlqQueueNames, expectedQueue, "Should have created queue: %s", expectedQueue)
		}
	})

	t.Run("Auto_Setup_Does_Not_Trigger_When_DLQ_Exists", func(t *testing.T) {
		// 1. Manually setup DLQ first
		req, _ := http.NewRequest("POST", "/api/dlq/setup", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 2. Get initial DLQ queue count
		queues, err := rabbitMQClient.GetQueuesFromAPI()
		require.NoError(t, err)

		initialDLQCount := 0
		for _, queue := range queues {
			if queueName, ok := queue["name"].(string); ok {
				if h.IsDLQQueue(queueName) {
					initialDLQCount++
				}
			}
		}

		// 3. Call DLQ list endpoint (should NOT trigger auto-setup)
		req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Verify DLQ queue count unchanged (no duplicate creation)
		queues, err = rabbitMQClient.GetQueuesFromAPI()
		require.NoError(t, err)

		finalDLQCount := 0
		for _, queue := range queues {
			if queueName, ok := queue["name"].(string); ok {
				if h.IsDLQQueue(queueName) {
					finalDLQCount++
				}
			}
		}

		assert.Equal(t, initialDLQCount, finalDLQCount, "DLQ queue count should remain unchanged when DLQ already exists")
	})
}

// TestDLQResilienceAfterReset tests DLQ auto-recovery after various reset scenarios
func TestDLQResilienceAfterReset(t *testing.T) {
	// Setup same as previous test
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
		Port:        "9001", // Test port
		WorkersURL:  "http://localhost:8001",
		PythonURL:   "http://localhost:8000",
	}

	rabbitMQClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)
	if _, err := rabbitMQClient.GetQueuesFromAPI(); err != nil {
		t.Skip("RabbitMQ not available for integration test")
	}

	workersClient := clients.NewWorkersClient(cfg.WorkersURL)
	pythonClient := clients.NewPythonClient(cfg.PythonURL)
	hub := websocket.NewHub()
	go hub.Run()
	h := &handlers.Handlers{
		RabbitMQClient: rabbitMQClient,
		WorkersClient:  workersClient,
		PythonClient:   pythonClient,
		WSHub:          hub,
		Config:         cfg,
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()

	dlq := router.Group("/api/dlq")
	{
		dlq.GET("/list", h.ListDLQMessages)
		dlq.POST("/setup", h.SetupDLQ)
	}
	router.POST("/api/reset", h.ResetGame)

	t.Run("DLQ_Recovers_After_Full_Reset", func(t *testing.T) {
		// 1. Setup DLQ initially
		req, _ := http.NewRequest("POST", "/api/dlq/setup", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 2. Verify DLQ exists
		req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var preResetResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &preResetResponse)
		require.NoError(t, err)
		assert.True(t, preResetResponse.Success)

		// 3. Perform full reset (clears all game queues including DLQ)
		req, _ = http.NewRequest("POST", "/api/reset", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Wait for reset to complete
		time.Sleep(1 * time.Second)

		// 4. Access DLQ again (should auto-setup)
		req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var postResetResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &postResetResponse)
		require.NoError(t, err)
		assert.True(t, postResetResponse.Success)

		// 5. Verify DLQ is available again (auto-setup worked)
		if data, ok := postResetResponse.Data.(map[string]interface{}); ok {
			if dlqQueues, exists := data["dlq_queues"]; exists {
				if queuesSlice, ok := dlqQueues.([]interface{}); ok {
					assert.GreaterOrEqual(t, len(queuesSlice), 4, "Should have auto-created DLQ queues after reset")
				}
			}
		}
	})

	t.Run("Multiple_Reset_Recovery_Cycles", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			// Reset
			req, _ := http.NewRequest("POST", "/api/reset", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			time.Sleep(500 * time.Millisecond)

			// Access DLQ (auto-setup)
			req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Cycle %d: DLQ list should work", i+1)

			var response models.APIResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err, "Cycle %d: Response should be valid JSON", i+1)
			assert.True(t, response.Success, "Cycle %d: Response should be successful", i+1)
		}
	})
}
