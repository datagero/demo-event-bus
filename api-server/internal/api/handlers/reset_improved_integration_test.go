package handlers

import (
	"bytes"
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

// TestImprovedResetFunctionality tests the enhanced reset function with proper error handling
func TestImprovedResetFunctionality(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := &config.Config{
		RabbitMQURL: "http://localhost:15672",
		WorkersURL:  "http://localhost:8001",
	}

	wsHub := websocket.NewHub()
	go wsHub.Run()

	h := &Handlers{
		WorkersClient:  clients.NewWorkersClient(cfg.WorkersURL),
		WSHub:          wsHub,
		Config:         cfg,
		RabbitMQClient: clients.NewRabbitMQClient(cfg.RabbitMQURL),
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/reset", h.ResetGame)
	router.POST("/api/workers/start", h.StartWorker)
	router.POST("/api/dlq/setup", h.SetupDLQ)
	router.GET("/api/rabbitmq/queues", h.GetRabbitMQQueues)
	router.GET("/api/rabbitmq/consumers", h.GetRabbitMQConsumers)
	router.GET("/api/workers/status", h.GetWorkersStatus)

	t.Run("Reset With Workers Service Available", func(t *testing.T) {
		// First, create some test resources

		// 1. Create a worker
		reqBody := map[string]interface{}{
			"player":           "test-reset-worker",
			"skills":           []string{"gather"},
			"fail_pct":         0.1,
			"speed_multiplier": 1.0,
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Skip("Cannot create worker for reset test - workers service may not be available")
		}

		// Wait for worker to be established
		time.Sleep(2 * time.Second)

		// 2. Set up DLQ infrastructure
		req, _ = http.NewRequest("POST", "/api/dlq/setup", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 3. Verify resources exist before reset
		req, _ = http.NewRequest("GET", "/api/rabbitmq/queues", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var queuesResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &queuesResponse)
		require.NoError(t, err)

		gameQueuesCount := 0
		if queuesResponse.Success {
			if queues, ok := queuesResponse.Data.([]interface{}); ok {
				for _, queue := range queues {
					if queueMap, ok := queue.(map[string]interface{}); ok {
						if name, exists := queueMap["name"]; exists {
							if nameStr, ok := name.(string); ok &&
								(nameStr == "game.skill.gather.q" ||
									nameStr == "game.dlq.failed.q" ||
									nameStr == "game.dlq.expired.q") {
								gameQueuesCount++
							}
						}
					}
				}
			}
		}

		assert.Greater(t, gameQueuesCount, 0, "Should have game queues before reset")

		// 4. Perform reset
		req, _ = http.NewRequest("POST", "/api/reset", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Reset should succeed when workers service is available")

		var resetResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &resetResponse)
		require.NoError(t, err)
		assert.True(t, resetResponse.Success, "Reset should be successful")

		// 5. Verify reset response contains detailed information
		resetData, ok := resetResponse.Data.(map[string]interface{})
		assert.True(t, ok, "Reset response should contain data")

		workersServiceAvailable, exists := resetData["workers_service_available"]
		assert.True(t, exists, "Should indicate workers service availability")
		assert.True(t, workersServiceAvailable.(bool), "Workers service should be available")

		workersStoppedCount, exists := resetData["workers_stopped_count"]
		assert.True(t, exists, "Should report workers stopped count")
		assert.GreaterOrEqual(t, int(workersStoppedCount.(float64)), 0, "Should report stopped workers count")

		queuesCount, exists := resetData["queues_count"]
		assert.True(t, exists, "Should report queues deleted count")
		assert.Greater(t, int(queuesCount.(float64)), 0, "Should have deleted some queues")

		hasErrors, exists := resetData["has_errors"]
		assert.True(t, exists, "Should indicate if there were errors")

				t.Logf("Reset had errors: %v", hasErrors)
		// Some exchange deletion errors are expected (405 Method Not Allowed)
		// but the overall operation should still be successful

		// 6. Verify cleanup - queues should be gone
		time.Sleep(1 * time.Second)

		req, _ = http.NewRequest("GET", "/api/rabbitmq/queues", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var finalQueuesResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &finalQueuesResponse)
		require.NoError(t, err)

		if finalQueuesResponse.Success {
			if queues, ok := finalQueuesResponse.Data.([]interface{}); ok {
				for _, queue := range queues {
					if queueMap, ok := queue.(map[string]interface{}); ok {
						if name, exists := queueMap["name"]; exists {
							if nameStr, ok := name.(string); ok {
								assert.NotContains(t, nameStr, "game.",
									"Game queues should be deleted after reset")
							}
						}
					}
				}
			}
		}
	})

	t.Run("Reset With Workers Service Unavailable", func(t *testing.T) {
		// Test reset behavior when workers service is not available
		// We'll simulate this by using a handler with a non-responsive WorkersClient

		badCfg := &config.Config{
			RabbitMQURL: "http://localhost:15672",
			WorkersURL:  "http://localhost:9999", // Non-existent port
		}

		badHandler := &Handlers{
			WorkersClient:  clients.NewWorkersClient(badCfg.WorkersURL),
			WSHub:          wsHub,
			Config:         badCfg,
			RabbitMQClient: clients.NewRabbitMQClient(badCfg.RabbitMQURL),
		}

		badRouter := gin.New()
		badRouter.POST("/api/reset", badHandler.ResetGame)

		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		badRouter.ServeHTTP(w, req)

		// Should still return a successful response but with warnings
		var resetResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &resetResponse)
		require.NoError(t, err)
		assert.True(t, resetResponse.Success, "Reset should be successful even with workers service unavailable")

		resetData, ok := resetResponse.Data.(map[string]interface{})
		assert.True(t, ok, "Reset response should contain data")

		workersServiceAvailable, exists := resetData["workers_service_available"]
		assert.True(t, exists, "Should indicate workers service availability")
		assert.False(t, workersServiceAvailable.(bool), "Workers service should be unavailable")

		workersStoppedStatus, exists := resetData["workers_stopped"]
		assert.True(t, exists, "Should report workers stopped status")
		assert.Equal(t, "service_unavailable", workersStoppedStatus.(string), "Should indicate service unavailable")

		hasErrors, exists := resetData["has_errors"]
		assert.True(t, exists, "Should indicate there were errors")
		assert.True(t, hasErrors.(bool), "Should have errors when workers service unavailable")

		errors, exists := resetData["errors"]
		assert.True(t, exists, "Should include error details")
		assert.NotNil(t, errors, "Should have error information")

		// Response should be 206 Partial Content when workers service is unavailable
		assert.Equal(t, http.StatusPartialContent, w.Code, "Should return partial content status when workers service unavailable")
	})

	t.Run("Reset Response Format Validation", func(t *testing.T) {
		// Test that the reset response contains all expected fields

		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var resetResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &resetResponse)
		require.NoError(t, err)

		resetData, ok := resetResponse.Data.(map[string]interface{})
		require.True(t, ok, "Reset response should contain data")

		// Check all required fields are present
		requiredFields := []string{
			"workers_stopped",
			"workers_stopped_count",
			"workers_service_available",
			"queues_deleted",
			"queues_count",
			"exchanges_deleted",
			"exchanges_count",
			"stats_cleared",
			"ui_reset",
			"errors",
			"has_errors",
		}

		for _, field := range requiredFields {
			_, exists := resetData[field]
			assert.True(t, exists, fmt.Sprintf("Reset response should contain field: %s", field))
		}

		// Validate field types
		assert.IsType(t, true, resetData["stats_cleared"], "stats_cleared should be boolean")
		assert.IsType(t, true, resetData["ui_reset"], "ui_reset should be boolean")
		assert.IsType(t, true, resetData["workers_service_available"], "workers_service_available should be boolean")
		assert.IsType(t, true, resetData["has_errors"], "has_errors should be boolean")
		assert.IsType(t, float64(0), resetData["workers_stopped_count"], "workers_stopped_count should be number")
		assert.IsType(t, float64(0), resetData["queues_count"], "queues_count should be number")
		assert.IsType(t, float64(0), resetData["exchanges_count"], "exchanges_count should be number")

		t.Logf("Reset completed with response: %+v", resetData)
	})
}
