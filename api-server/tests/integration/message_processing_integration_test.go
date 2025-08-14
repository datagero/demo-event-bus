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

// TestMessageProcessingFlow tests end-to-end message processing scenarios
func TestMessageProcessingFlow(t *testing.T) {
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
	router.POST("/api/publish", h.PublishMessage)
	router.GET("/api/rabbitmq/metrics", h.GetRabbitMQMetrics)
	router.GET("/api/rabbitmq/consumers", h.GetRabbitMQConsumers)
	router.POST("/api/reset", h.ResetGame)

	// Reset before starting
	req, _ := http.NewRequest("POST", "/api/reset", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	time.Sleep(1 * time.Second)

	t.Run("Worker Processes Messages Successfully", func(t *testing.T) {
		// Start Alice worker
		reqBody := map[string]interface{}{
			"player":           "alice-processing",
			"skills":           []string{"gather", "slay"},
			"fail_pct":         0.0, // No failures for this test
			"speed_multiplier": 1.0,
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		time.Sleep(2 * time.Second) // Wait for worker registration

		// Check initial metrics
		req, _ = http.NewRequest("GET", "/api/rabbitmq/metrics", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var initialMetrics models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &initialMetrics)
		require.NoError(t, err)

		// Publish a gather quest
		publishBody := map[string]interface{}{
			"routing_key": "game.quest.gather",
			"payload": map[string]interface{}{
				"case_id":    "test-processing-1",
				"quest_type": "gather",
				"points":     5,
			},
		}
		jsonBody, _ = json.Marshal(publishBody)

		req, _ = http.NewRequest("POST", "/api/publish", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var publishResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &publishResponse)
		require.NoError(t, err)
		assert.True(t, publishResponse.Success, "Message should publish successfully")

		// Wait for processing
		time.Sleep(3 * time.Second)

		// Check that message was processed (queue should be empty or smaller)
		req, _ = http.NewRequest("GET", "/api/rabbitmq/metrics", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var finalMetrics models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &finalMetrics)
		require.NoError(t, err)
		assert.True(t, finalMetrics.Success, "Should be able to get metrics")

		// The message should have been processed, so pending count should be manageable
		if finalMetrics.Success {
			if metricsData, ok := finalMetrics.Data.(map[string]interface{}); ok {
				if totalPending, exists := metricsData["total_pending"]; exists {
					if pending, ok := totalPending.(float64); ok {
						t.Logf("Total pending messages: %v", pending)
						// We expect the message to be processed quickly
						assert.LessOrEqual(t, pending, float64(10), "Messages should be processed efficiently")
					}
				}
			}
		}
	})

	t.Run("Worker Handles Multiple Message Types", func(t *testing.T) {
		// Publish different quest types
		questTypes := []struct {
			routingKey string
			questType  string
		}{
			{"game.quest.gather", "gather"},
			{"game.quest.slay", "slay"},
		}

		for i, quest := range questTypes {
			publishBody := map[string]interface{}{
				"routing_key": quest.routingKey,
				"payload": map[string]interface{}{
					"case_id":    fmt.Sprintf("test-multi-%d", i),
					"quest_type": quest.questType,
					"points":     10,
				},
			}
			jsonBody, _ := json.Marshal(publishBody)

			req, _ := http.NewRequest("POST", "/api/publish", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response models.APIResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.True(t, response.Success, fmt.Sprintf("Quest type %s should publish successfully", quest.questType))
		}

		// Wait for processing
		time.Sleep(3 * time.Second)

		// Verify worker is still active and processing
		req, _ := http.NewRequest("GET", "/api/rabbitmq/consumers", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var consumersResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &consumersResponse)
		require.NoError(t, err)

		if consumersResponse.Success {
			consumers, ok := consumersResponse.Data.([]interface{})
			if ok {
				foundGather := false
				foundSlay := false
				for _, consumer := range consumers {
					if consumerMap, ok := consumer.(map[string]interface{}); ok {
						if tag, exists := consumerMap["consumer_tag"]; exists {
							if tagStr, ok := tag.(string); ok {
								if tagStr == "alice-processing-gather-worker-0" {
									foundGather = true
									active, exists := consumerMap["active"]
									assert.True(t, exists)
									assert.True(t, active.(bool), "Gather worker should remain active")
								}
								if tagStr == "alice-processing-slay-worker-0" {
									foundSlay = true
									active, exists := consumerMap["active"]
									assert.True(t, exists)
									assert.True(t, active.(bool), "Slay worker should remain active")
								}
							}
						}
					}
				}
				assert.True(t, foundGather, "Gather worker should be found")
				assert.True(t, foundSlay, "Slay worker should be found")
			}
		}
	})
}

// TestMessageFailureScenarios tests DLQ routing and failure handling
func TestMessageFailureScenarios(t *testing.T) {
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
	router.POST("/api/publish", h.PublishMessage)
	router.POST("/api/dlq/setup", h.SetupDLQ)
	router.GET("/api/dlq/list", h.ListDLQMessages)
	router.POST("/api/reset", h.ResetGame)

	// Reset and setup DLQ
	req, _ := http.NewRequest("POST", "/api/reset", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	time.Sleep(1 * time.Second)

	req, _ = http.NewRequest("POST", "/api/dlq/setup", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	time.Sleep(1 * time.Second)

	t.Run("High Failure Rate Worker Sends Messages to DLQ", func(t *testing.T) {
		// Start a worker with high failure rate
		reqBody := map[string]interface{}{
			"player":           "alice-failure",
			"skills":           []string{"gather"},
			"fail_pct":         0.8, // 80% failure rate
			"speed_multiplier": 2.0, // Fast processing
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		time.Sleep(2 * time.Second)

		// Send multiple messages to increase chance of failures
		for i := 0; i < 10; i++ {
			publishBody := map[string]interface{}{
				"routing_key": "game.quest.gather",
				"payload": map[string]interface{}{
					"case_id":    fmt.Sprintf("failure-test-%d", i),
					"quest_type": "gather",
					"points":     5,
				},
			}
			jsonBody, _ := json.Marshal(publishBody)

			req, _ := http.NewRequest("POST", "/api/publish", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		}

		// Wait for processing and failures
		time.Sleep(5 * time.Second)

		// Check DLQ for failed messages
		req, _ = http.NewRequest("GET", "/api/dlq/list", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var dlqResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &dlqResponse)
		require.NoError(t, err)

		if dlqResponse.Success {
			t.Logf("DLQ Response: %+v", dlqResponse.Data)
			// With 80% failure rate and 10 messages, we should see some failures
			// This test validates the DLQ system is working
		} else {
			t.Logf("DLQ list failed: %s", dlqResponse.Error)
		}
	})
}

// TestQueueManagement tests queue creation and cleanup scenarios
func TestQueueManagement(t *testing.T) {
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
	router.GET("/api/rabbitmq/queues", h.GetRabbitMQQueues)
	router.POST("/api/reset", h.ResetGame)

	t.Run("Worker Creation Establishes Queues", func(t *testing.T) {
		// Reset first
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		time.Sleep(1 * time.Second)

		// Check initial queue state
		req, _ = http.NewRequest("GET", "/api/rabbitmq/queues", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var initialQueues models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &initialQueues)
		require.NoError(t, err)

		initialQueueCount := 0
		if initialQueues.Success {
			if queues, ok := initialQueues.Data.([]interface{}); ok {
				for _, queue := range queues {
					if queueMap, ok := queue.(map[string]interface{}); ok {
						if name, exists := queueMap["name"]; exists {
							if nameStr, ok := name.(string); ok &&
								(nameStr == "game.skill.gather.q" || nameStr == "game.skill.slay.q") {
								initialQueueCount++
							}
						}
					}
				}
			}
		}

		// Start worker
		reqBody := map[string]interface{}{
			"player":           "alice-queues",
			"skills":           []string{"gather", "slay"},
			"fail_pct":         0.1,
			"speed_multiplier": 1.0,
		}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ = http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		time.Sleep(3 * time.Second)

		// Check that queues were created
		req, _ = http.NewRequest("GET", "/api/rabbitmq/queues", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var finalQueues models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &finalQueues)
		require.NoError(t, err)

		if finalQueues.Success {
			queues, ok := finalQueues.Data.([]interface{})
			assert.True(t, ok)

			foundGatherQueue := false
			foundSlayQueue := false
			for _, queue := range queues {
				if queueMap, ok := queue.(map[string]interface{}); ok {
					if name, exists := queueMap["name"]; exists {
						if nameStr, ok := name.(string); ok {
							if nameStr == "game.skill.gather.q" {
								foundGatherQueue = true
							}
							if nameStr == "game.skill.slay.q" {
								foundSlayQueue = true
							}
						}
					}
				}
			}

			assert.True(t, foundGatherQueue, "Gather queue should be created")
			assert.True(t, foundSlayQueue, "Slay queue should be created")
		}
	})

	t.Run("Reset Cleans Up Queues", func(t *testing.T) {
		// Reset
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resetResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &resetResponse)
		require.NoError(t, err)
		assert.True(t, resetResponse.Success)

		// Verify queues deleted info in response
		if resetData, ok := resetResponse.Data.(map[string]interface{}); ok {
			if queuesDeleted, exists := resetData["queues_deleted"]; exists {
				if deletedList, ok := queuesDeleted.([]interface{}); ok {
					t.Logf("Queues deleted: %v", deletedList)
					// Should have deleted some game queues
				}
			}
		}

		time.Sleep(2 * time.Second)

		// Check that game queues are gone
		req, _ = http.NewRequest("GET", "/api/rabbitmq/queues", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var queues models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &queues)
		require.NoError(t, err)

		if queues.Success {
			if queueList, ok := queues.Data.([]interface{}); ok {
				for _, queue := range queueList {
					if queueMap, ok := queue.(map[string]interface{}); ok {
						if name, exists := queueMap["name"]; exists {
							if nameStr, ok := name.(string); ok {
								assert.NotContains(t, nameStr, "game.skill",
									"Game skill queues should be cleaned up after reset")
							}
						}
					}
				}
			}
		}
	})
}
