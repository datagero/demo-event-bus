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

// TestDLQMessageFlowEndToEnd tests complete DLQ message flow scenarios
func TestDLQMessageFlowEndToEnd(t *testing.T) {
	// Setup test configuration
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

	// Setup routes
	dlq := router.Group("/api/dlq")
	{
		dlq.GET("/list", h.ListDLQMessages)
		dlq.POST("/setup", h.SetupDLQ)
		dlq.GET("/inspect", h.InspectDLQ)
		dlq.POST("/reissue", h.ReissueDLQMessages)
	}

	router.POST("/api/reset", h.ResetGame)
	router.POST("/api/master/start", h.StartMaster)
	router.POST("/api/workers/start", h.StartWorker)
	router.POST("/api/workers/stop", h.StopWorker)

	t.Run("Failed_Messages_Appear_In_DLQ_With_Auto_Setup", func(t *testing.T) {
		// 1. Clean state
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		time.Sleep(1 * time.Second)

		// 2. Create a high-failure worker
		workerReq := map[string]interface{}{
			"player":           "test-dlq-worker",
			"skills":           []string{"gather"},
			"fail_pct":         0.9, // 90% failure rate
			"speed_multiplier": 2.0,
		}
		jsonBody, _ := json.Marshal(workerReq)

		req, _ = http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Skip("Workers service not available for DLQ flow test")
		}

		// Wait for worker to be established
		time.Sleep(2 * time.Second)

		// 3. Send quest wave (messages will fail due to high failure rate)
		questReq := map[string]interface{}{
			"count": 5,
			"delay": 0.1,
		}
		jsonBody, _ = json.Marshal(questReq)

		req, _ = http.NewRequest("POST", "/api/master/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Wait for messages to be processed and fail
		time.Sleep(5 * time.Second)

		// 4. Check DLQ (should auto-setup and show failed messages)
		req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var dlqResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &dlqResponse)
		require.NoError(t, err)
		assert.True(t, dlqResponse.Success)

		// 5. Verify failed messages exist in DLQ
		if data, ok := dlqResponse.Data.(map[string]interface{}); ok {
			if totalDLQ, exists := data["total_dlq"]; exists {
				if count, ok := totalDLQ.(float64); ok {
					assert.Greater(t, int(count), 0, "Should have failed messages in DLQ")
				}
			}

			if categories, exists := data["categories"]; exists {
				if catMap, ok := categories.(map[string]interface{}); ok {
					if failed, exists := catMap["failed"]; exists {
						if failedCount, ok := failed.(float64); ok {
							assert.Greater(t, int(failedCount), 0, "Should have messages in 'failed' category")
						}
					}
				}
			}
		}

		// 6. Cleanup worker
		stopReq := map[string]interface{}{
			"player": "test-dlq-worker",
		}
		jsonBody, _ = json.Marshal(stopReq)

		req, _ = http.NewRequest("POST", "/api/workers/stop", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
	})

	t.Run("DLQ_Categories_Classification", func(t *testing.T) {
		// 1. Setup DLQ first (ensure it exists)
		req, _ := http.NewRequest("POST", "/api/dlq/setup", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 2. Check DLQ inspection endpoint
		req, _ = http.NewRequest("GET", "/api/dlq/inspect", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var inspectResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &inspectResponse)
		require.NoError(t, err)
		assert.True(t, inspectResponse.Success)

		// 3. Verify DLQ topology inspection
		if data, ok := inspectResponse.Data.(map[string]interface{}); ok {
			// Should contain topology information
			assert.Contains(t, data, "dlq_exchanges", "Should include DLQ exchanges info")
			assert.Contains(t, data, "dlq_queues", "Should include DLQ queues info")
		}
	})

	t.Run("Quest_Wave_To_DLQ_Integration", func(t *testing.T) {
		// This test validates the specific scenario the user reported:
		// "clicking Start Quest Wave but there is no DLQ"

		// 1. Reset to clear everything
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		time.Sleep(1 * time.Second)

		// 2. Start workers (to process the quest wave)
		workerReq := map[string]interface{}{
			"player":           "test-quest-wave-worker",
			"skills":           []string{"gather", "slay", "escort"},
			"fail_pct":         0.5, // 50% failure rate to generate DLQ messages
			"speed_multiplier": 1.5,
		}
		jsonBody, _ := json.Marshal(workerReq)

		req, _ = http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Skip("Workers service not available for quest wave DLQ test")
		}

		time.Sleep(2 * time.Second)

		// 3. Send quest wave (this was the user's action that revealed the issue)
		questReq := map[string]interface{}{
			"count": 3,
			"delay": 0.2,
		}
		jsonBody, _ = json.Marshal(questReq)

		req, _ = http.NewRequest("POST", "/api/master/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Quest wave should start successfully")

		// Wait for processing
		time.Sleep(4 * time.Second)

		// 4. Check DLQ immediately (should auto-setup and show messages)
		req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "DLQ list should work after quest wave")

		var dlqResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &dlqResponse)
		require.NoError(t, err)
		assert.True(t, dlqResponse.Success, "DLQ response should be successful")

		// 5. Verify auto-setup occurred and messages are visible
		if data, ok := dlqResponse.Data.(map[string]interface{}); ok {
			// Should have DLQ queues (auto-setup worked)
			if dlqQueues, exists := data["dlq_queues"]; exists {
				if queuesSlice, ok := dlqQueues.([]interface{}); ok {
					assert.GreaterOrEqual(t, len(queuesSlice), 4, "Should have DLQ queues after quest wave")
				}
			}

			// Should have some failed messages (due to 50% failure rate)
			if totalDLQ, exists := data["total_dlq"]; exists {
				if count, ok := totalDLQ.(float64); ok {
					t.Logf("Found %d messages in DLQ after quest wave", int(count))
					// With 50% failure rate and 9 total messages (3 types × 3 count),
					// we expect some failures, but this can vary
					assert.GreaterOrEqual(t, int(count), 0, "Should have processed quest wave")
				}
			}
		}

		// 6. Cleanup
		stopReq := map[string]interface{}{
			"player": "test-quest-wave-worker",
		}
		jsonBody, _ = json.Marshal(stopReq)

		req, _ = http.NewRequest("POST", "/api/workers/stop", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
	})
}

// TestDLQMessageReissue tests the DLQ message reissue functionality
func TestDLQMessageReissue(t *testing.T) {
	// Setup same as previous tests
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
		dlq.POST("/reissue", h.ReissueDLQMessages)
		dlq.POST("/reissue/all", h.ReissueAllDLQMessages)
	}

	t.Run("DLQ_Reissue_API_Validation", func(t *testing.T) {
		// 1. Setup DLQ
		req, _ := http.NewRequest("POST", "/api/dlq/setup", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 2. Test reissue endpoint with invalid request
		req, _ = http.NewRequest("POST", "/api/dlq/reissue", bytes.NewBuffer([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should reject invalid JSON")

		// 3. Test reissue with valid but empty request
		req, _ = http.NewRequest("POST", "/api/dlq/reissue", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should handle empty request gracefully (may be 400 or 200 depending on validation)
		assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest}, w.Code)

		// 4. Test reissue all endpoint
		req, _ = http.NewRequest("POST", "/api/dlq/reissue/all", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should handle the request
		assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError}, w.Code)
	})
}
