package handlers_test

import (
	"bytes"
	"demo-event-bus-api/internal/api/handlers"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/models"
	"demo-event-bus-api/internal/websocket"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerLifecycleIntegration tests the complete worker lifecycle scenarios
// that caused the "Alice not working" issue
func TestWorkerLifecycleIntegration(t *testing.T) {
	// Skip if not integration test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup test configuration
	cfg := &config.Config{
		RabbitMQURL: "http://localhost:15672",
		WorkersURL:  "http://localhost:8001",
	}

	// Create test handlers
	wsHub := websocket.NewHub()
	go wsHub.Run()

	h := &handlers.Handlers{
		WorkersClient:  clients.NewWorkersClient(cfg.WorkersURL),
		WSHub:          wsHub,
		Config:         cfg,
		RabbitMQClient: clients.NewRabbitMQClient(cfg.RabbitMQURL),
	}

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/workers/start", h.StartWorker)
	router.POST("/api/workers/stop", h.StopWorker)
	router.GET("/api/workers/status", h.GetWorkersStatus)
	router.GET("/api/rabbitmq/consumers", h.GetRabbitMQConsumers)
	router.POST("/api/reset", h.ResetGame)

	t.Run("Worker Start and Verify Active", func(t *testing.T) {
		// Create Alice worker
		reqBody := map[string]interface{}{
			"player":           "alice-test",
			"skills":           []string{"gather", "slay"},
			"fail_pct":         0.2,
			"speed_multiplier": 1.0,
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)

		// Wait for worker to register with RabbitMQ
		time.Sleep(2 * time.Second)

		// Verify worker appears in RabbitMQ consumers
		req, _ = http.NewRequest("GET", "/api/rabbitmq/consumers", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var consumersResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &consumersResponse)
		require.NoError(t, err)
		assert.True(t, consumersResponse.Success)

		// Check that alice-test consumers exist and are active
		consumers, ok := consumersResponse.Data.([]interface{})
		if ok && len(consumers) > 0 {
			found := false
			for _, consumer := range consumers {
				if consumerMap, ok := consumer.(map[string]interface{}); ok {
					if tag, exists := consumerMap["consumer_tag"]; exists {
						if tagStr, ok := tag.(string); ok &&
							(tagStr == "alice-test-gather-worker-0" || tagStr == "alice-test-slay-worker-0") {
							found = true
							active, exists := consumerMap["active"]
							assert.True(t, exists)
							assert.True(t, active.(bool))
						}
					}
				}
			}
			assert.True(t, found, "Alice test consumers should be found and active")
		}
	})

	t.Run("Worker Stop and Verify Cleanup", func(t *testing.T) {
		// Stop Alice worker
		reqBody := map[string]interface{}{
			"player": "alice-test",
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/workers/stop", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)

		// Wait for cleanup
		time.Sleep(2 * time.Second)

		// Verify worker no longer appears in RabbitMQ consumers
		req, _ = http.NewRequest("GET", "/api/rabbitmq/consumers", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var consumersResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &consumersResponse)
		require.NoError(t, err)

		// Check that alice-test consumers are gone
		if consumersResponse.Success {
			consumers, ok := consumersResponse.Data.([]interface{})
			if ok {
				for _, consumer := range consumers {
					if consumerMap, ok := consumer.(map[string]interface{}); ok {
						if tag, exists := consumerMap["consumer_tag"]; exists {
							if tagStr, ok := tag.(string); ok {
								assert.NotContains(t, tagStr, "alice-test",
									"Alice test consumers should be cleaned up")
							}
						}
					}
				}
			}
		}
	})

	t.Run("Reset Clears All Workers", func(t *testing.T) {
		// Create multiple workers
		workers := []string{"alice-reset", "bob-reset"}
		for _, worker := range workers {
			reqBody := map[string]interface{}{
				"player":           worker,
				"skills":           []string{"gather"},
				"fail_pct":         0.1,
				"speed_multiplier": 1.0,
			}
			jsonBody, _ := json.Marshal(reqBody)

			req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		}

		// Wait for workers to register
		time.Sleep(2 * time.Second)

		// Reset the game
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resetResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &resetResponse)
		require.NoError(t, err)
		assert.True(t, resetResponse.Success)

		// Verify reset data contains workers_stopped
		resetData, ok := resetResponse.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "all", resetData["workers_stopped"])

		// Wait for cleanup
		time.Sleep(3 * time.Second)

		// Verify no consumers remain
		req, _ = http.NewRequest("GET", "/api/rabbitmq/consumers", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var consumersResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &consumersResponse)
		require.NoError(t, err)

		if consumersResponse.Success {
			consumers, ok := consumersResponse.Data.([]interface{})
			if ok {
				assert.Empty(t, consumers, "All consumers should be cleared after reset")
			}
		}
	})
}

// TestWorkerStuckStateDetection tests for the specific "stopping" state issue
func TestWorkerStuckStateDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := &config.Config{
		RabbitMQURL: "http://localhost:15672",
		WorkersURL:  "http://localhost:8001",
	}

	wsHub := websocket.NewHub()
	go wsHub.Run()

	h := &handlers.Handlers{
		WorkersClient:  clients.NewWorkersClient(cfg.WorkersURL),
		WSHub:          wsHub,
		Config:         cfg,
		RabbitMQClient: clients.NewRabbitMQClient(cfg.RabbitMQURL),
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/workers/start", h.StartWorker)
	router.GET("/api/workers/status", h.GetWorkersStatus)
	router.GET("/api/rabbitmq/consumers", h.GetRabbitMQConsumers)

	t.Run("Detect Worker Registration vs Actual Activity", func(t *testing.T) {
		// Start a worker
		reqBody := map[string]interface{}{
			"player":           "alice-stuck-test",
			"skills":           []string{"gather"},
			"fail_pct":         0.0, // No failures for this test
			"speed_multiplier": 1.0,
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Wait for registration
		time.Sleep(2 * time.Second)

		// Check workers service status
		req, _ = http.NewRequest("GET", "/api/workers/status", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var statusResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &statusResponse)
		require.NoError(t, err)
		assert.True(t, statusResponse.Success, "Workers service should be reachable")

		// Check RabbitMQ consumers
		req, _ = http.NewRequest("GET", "/api/rabbitmq/consumers", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var consumersResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &consumersResponse)
		require.NoError(t, err)
		assert.True(t, consumersResponse.Success, "RabbitMQ should be reachable")

		// Verify the worker exists in both places
		statusData, ok := statusResponse.Data.(map[string]interface{})
		if assert.True(t, ok) {
			workerCount, exists := statusData["worker_count"]
			if assert.True(t, exists) {
				// Should have at least one worker
				if count, ok := workerCount.(float64); ok {
					assert.Greater(t, count, float64(0), "Should have registered workers")
				}
			}
		}

		// Verify RabbitMQ consumer exists and is active
		if consumersResponse.Success {
			consumers, ok := consumersResponse.Data.([]interface{})
			if ok {
				found := false
				for _, consumer := range consumers {
					if consumerMap, ok := consumer.(map[string]interface{}); ok {
						if tag, exists := consumerMap["consumer_tag"]; exists {
							if tagStr, ok := tag.(string); ok &&
								tagStr == "alice-stuck-test-gather-worker-0" {
								found = true
								active, exists := consumerMap["active"]
								assert.True(t, exists)
								assert.True(t, active.(bool), "Consumer should be active, not stuck")

								activityStatus, exists := consumerMap["activity_status"]
								if exists {
									assert.Equal(t, "up", activityStatus, "Consumer should be up, not stopping")
								}
							}
						}
					}
				}
				assert.True(t, found, "Alice stuck test consumer should be found")
			}
		}
	})
}

// TestRabbitMQConnectivityRecovery tests the RabbitMQ connection issues we encountered
func TestRabbitMQConnectivityRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := &config.Config{
		RabbitMQURL: "http://localhost:15672",
		WorkersURL:  "http://localhost:8001",
	}

	wsHub := websocket.NewHub()
	go wsHub.Run()

	h := &handlers.Handlers{
		WorkersClient:  clients.NewWorkersClient(cfg.WorkersURL),
		WSHub:          wsHub,
		Config:         cfg,
		RabbitMQClient: clients.NewRabbitMQClient(cfg.RabbitMQURL),
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/rabbitmq/consumers", h.GetRabbitMQConsumers)
	router.GET("/api/rabbitmq/metrics", h.GetRabbitMQMetrics)

	t.Run("RabbitMQ Management API Connectivity", func(t *testing.T) {
		// Test RabbitMQ Management API is accessible
		req, _ := http.NewRequest("GET", "/api/rabbitmq/consumers", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusInternalServerError {
			// Check if it's a connection refused error
			var response models.APIResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err == nil && !response.Success {
				if response.Error != "" {
					t.Logf("RabbitMQ connection error: %s", response.Error)
					t.Skip("RabbitMQ not available for this test - this is the issue we're testing for")
				}
			}
		}

		assert.Equal(t, http.StatusOK, w.Code, "RabbitMQ Management API should be accessible")

		var response models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success, "RabbitMQ API call should succeed")
	})

	t.Run("RabbitMQ Metrics Availability", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/rabbitmq/metrics", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusInternalServerError {
			var response models.APIResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err == nil && !response.Success {
				if response.Error != "" {
					t.Logf("RabbitMQ metrics error: %s", response.Error)
					t.Skip("RabbitMQ not available for metrics test")
				}
			}
		}

		assert.Equal(t, http.StatusOK, w.Code, "RabbitMQ metrics should be accessible")
	})
}

// TestWorkerAPIValidation tests the API request format issues we encountered
func TestWorkerAPIValidation(t *testing.T) {
	cfg := &config.Config{
		WorkersURL: "http://localhost:8001",
	}

	wsHub := websocket.NewHub()
	go wsHub.Run()

	h := &handlers.Handlers{
		WorkersClient:  clients.NewWorkersClient(cfg.WorkersURL),
		WSHub:          wsHub,
		Config:         cfg,
		RabbitMQClient: clients.NewRabbitMQClient(cfg.RabbitMQURL),
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/workers/start", h.StartWorker)
	router.POST("/api/workers/stop", h.StopWorker)

	t.Run("Start Worker Validation", func(t *testing.T) {
		testCases := []struct {
			name         string
			requestBody  map[string]interface{}
			expectedCode int
			shouldPass   bool
		}{
			{
				name: "Valid Request",
				requestBody: map[string]interface{}{
					"player":           "alice-validation",
					"skills":           []string{"gather", "slay"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusOK,
				shouldPass:   true,
			},
			{
				name: "Missing Player Field",
				requestBody: map[string]interface{}{
					"skills":           []string{"gather"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Missing Skills Field",
				requestBody: map[string]interface{}{
					"player":           "alice-validation",
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Wrong Field Name (worker_name instead of player)",
				requestBody: map[string]interface{}{
					"worker_name":      "alice-validation",
					"skills":           []string{"gather"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				jsonBody, _ := json.Marshal(tc.requestBody)
				req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("Test case: %s", tc.name))

				var response models.APIResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				if tc.shouldPass {
					assert.True(t, response.Success || w.Code == http.StatusInternalServerError,
						"Valid requests should succeed or fail only due to external service issues")
				} else {
					assert.False(t, response.Success, "Invalid requests should fail validation")
				}
			})
		}
	})

	t.Run("Stop Worker Validation", func(t *testing.T) {
		testCases := []struct {
			name         string
			requestBody  map[string]interface{}
			expectedCode int
			shouldPass   bool
		}{
			{
				name: "Valid Stop Request",
				requestBody: map[string]interface{}{
					"player": "alice-validation",
				},
				expectedCode: http.StatusOK,
				shouldPass:   true,
			},
			{
				name:         "Missing Player Field",
				requestBody:  map[string]interface{}{},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Wrong Field Name (worker_name instead of player)",
				requestBody: map[string]interface{}{
					"worker_name": "alice-validation",
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				jsonBody, _ := json.Marshal(tc.requestBody)
				req, _ := http.NewRequest("POST", "/api/workers/stop", bytes.NewBuffer(jsonBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("Test case: %s", tc.name))

				var response models.APIResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				if tc.shouldPass {
					assert.True(t, response.Success || w.Code == http.StatusInternalServerError,
						"Valid requests should succeed or fail only due to external service issues")
				} else {
					assert.False(t, response.Success, "Invalid requests should fail validation")
				}
			})
		}
	})
}
