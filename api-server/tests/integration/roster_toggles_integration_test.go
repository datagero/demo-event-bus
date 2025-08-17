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

// TestRosterTogglesIntegration tests complete roster toggle functionality with workers service
func TestRosterTogglesIntegration(t *testing.T) {
	// Skip if not integration test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup test configuration
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
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
	router.POST("/api/workers/control", h.ControlWorker)
	router.POST("/api/player/control", h.ControlPlayer)
	router.POST("/api/player/delete", h.DeletePlayer)
	router.GET("/api/workers/status", h.GetWorkersStatus)
	router.GET("/api/rabbitmq/consumers", h.GetRabbitMQConsumers)
	router.POST("/api/reset", h.ResetGame)

	// Unique test player name to avoid conflicts
	testPlayer := fmt.Sprintf("roster-test-%d", time.Now().Unix())

	t.Run("Complete Roster Toggle Lifecycle", func(t *testing.T) {
		// Step 1: Start a worker
		startReq := map[string]interface{}{
			"player":           testPlayer,
			"skills":           []string{"gather", "slay"},
			"fail_pct":         0.1,
			"speed_multiplier": 1.0,
			"workers":          1,
		}
		startBody, _ := json.Marshal(startReq)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(startBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Failed to start worker: %s", w.Body.String())

		var startResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &startResponse)
		require.NoError(t, err)
		assert.True(t, startResponse.Success)

		// Wait for worker to be active
		time.Sleep(2 * time.Second)

		// Step 2: Verify worker is active in status
		req, _ = http.NewRequest("GET", "/api/workers/status", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var statusResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &statusResponse)
		require.NoError(t, err)
		assert.True(t, statusResponse.Success)

		// Check that our test player appears in the status
		statusData, ok := statusResponse.Data.(map[string]interface{})
		require.True(t, ok, "Status data should be a map")
		workers, ok := statusData["workers"].([]interface{})
		require.True(t, ok, "Workers should be an array")

		workerFound := false
		for _, worker := range workers {
			if workerName, ok := worker.(string); ok && workerName == testPlayer {
				workerFound = true
				break
			}
		}
		assert.True(t, workerFound, "Test player %s should appear in workers list", testPlayer)

		// Step 3: Test pause functionality
		pauseReq := map[string]interface{}{
			"player": testPlayer,
			"action": "pause",
		}
		pauseBody, _ := json.Marshal(pauseReq)

		req, _ = http.NewRequest("POST", "/api/workers/control", bytes.NewBuffer(pauseBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Failed to pause worker: %s", w.Body.String())

		var pauseResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &pauseResponse)
		require.NoError(t, err)
		assert.True(t, pauseResponse.Success)
		assert.Contains(t, pauseResponse.Message, "executed successfully")

		// Step 4: Test resume functionality
		resumeReq := map[string]interface{}{
			"player": testPlayer,
			"action": "resume",
		}
		resumeBody, _ := json.Marshal(resumeReq)

		req, _ = http.NewRequest("POST", "/api/workers/control", bytes.NewBuffer(resumeBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Failed to resume worker: %s", w.Body.String())

		var resumeResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &resumeResponse)
		require.NoError(t, err)
		assert.True(t, resumeResponse.Success)
		assert.Contains(t, resumeResponse.Message, "executed successfully")

		// Step 5: Test delete functionality (via DeletePlayer)
		deleteReq := map[string]interface{}{
			"player": testPlayer,
		}
		deleteBody, _ := json.Marshal(deleteReq)

		req, _ = http.NewRequest("POST", "/api/player/delete", bytes.NewBuffer(deleteBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Failed to delete player: %s", w.Body.String())

		var deleteResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &deleteResponse)
		require.NoError(t, err)
		assert.True(t, deleteResponse.Success)
		assert.Contains(t, deleteResponse.Message, "deleted successfully")

		// Step 6: Verify worker is removed from status
		time.Sleep(2 * time.Second)

		req, _ = http.NewRequest("GET", "/api/workers/status", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &statusResponse)
		require.NoError(t, err)
		assert.True(t, statusResponse.Success)

		// Check that our test player no longer appears in the status
		statusData, ok = statusResponse.Data.(map[string]interface{})
		require.True(t, ok, "Status data should be a map")
		workers, ok = statusData["workers"].([]interface{})
		require.True(t, ok, "Workers should be an array")

		workerFound = false
		for _, worker := range workers {
			if workerName, ok := worker.(string); ok && workerName == testPlayer {
				workerFound = true
				break
			}
		}
		assert.False(t, workerFound, "Test player %s should no longer appear in workers list", testPlayer)
	})

	t.Run("Pause and Resume via Player Control", func(t *testing.T) {
		// Unique test player name
		testPlayer := fmt.Sprintf("player-control-test-%d", time.Now().Unix())

		// Start a worker first
		startReq := map[string]interface{}{
			"player":           testPlayer,
			"skills":           []string{"gather"},
			"fail_pct":         0.0,
			"speed_multiplier": 1.0,
			"workers":          1,
		}
		startBody, _ := json.Marshal(startReq)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(startBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		time.Sleep(2 * time.Second)

		// Test pause via player control
		pauseReq := map[string]interface{}{
			"player": testPlayer,
			"action": "pause",
		}
		pauseBody, _ := json.Marshal(pauseReq)

		req, _ = http.NewRequest("POST", "/api/player/control", bytes.NewBuffer(pauseBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Failed to pause via player control: %s", w.Body.String())

		var pauseResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &pauseResponse)
		require.NoError(t, err)
		assert.True(t, pauseResponse.Success)

		// Test resume via player control
		resumeReq := map[string]interface{}{
			"player": testPlayer,
			"action": "resume",
		}
		resumeBody, _ := json.Marshal(resumeReq)

		req, _ = http.NewRequest("POST", "/api/player/control", bytes.NewBuffer(resumeBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Failed to resume via player control: %s", w.Body.String())

		var resumeResponse models.APIResponse
		err = json.Unmarshal(w.Body.Bytes(), &resumeResponse)
		require.NoError(t, err)
		assert.True(t, resumeResponse.Success)

		// Cleanup
		deleteReq := map[string]interface{}{
			"player": testPlayer,
		}
		deleteBody, _ := json.Marshal(deleteReq)

		req, _ = http.NewRequest("POST", "/api/player/delete", bytes.NewBuffer(deleteBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Crash Action via Control (chaos disabled)", func(t *testing.T) {
		// Unique test player name
		testPlayer := fmt.Sprintf("crash-test-%d", time.Now().Unix())

		// Start a worker first
		startReq := map[string]interface{}{
			"player":           testPlayer,
			"skills":           []string{"gather"},
			"fail_pct":         0.0,
			"speed_multiplier": 1.0,
			"workers":          1,
		}
		startBody, _ := json.Marshal(startReq)

		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(startBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		time.Sleep(2 * time.Second)

		// Test crash action (expect failure since chaos is disabled)
		crashReq := map[string]interface{}{
			"player": testPlayer,
			"action": "crash",
		}
		crashBody, _ := json.Marshal(crashReq)

		req, _ = http.NewRequest("POST", "/api/player/control", bytes.NewBuffer(crashBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Expect failure due to chaos being disabled
		assert.Equal(t, http.StatusInternalServerError, w.Code, "Crash action should fail when chaos is disabled")

		var crashResponse models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &crashResponse)
		require.NoError(t, err)
		assert.False(t, crashResponse.Success)
		assert.Contains(t, crashResponse.Error, "chaos is disabled")

		// Cleanup
		deleteReq := map[string]interface{}{
			"player": testPlayer,
		}
		deleteBody, _ := json.Marshal(deleteReq)

		req, _ = http.NewRequest("POST", "/api/player/delete", bytes.NewBuffer(deleteBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Invalid Player Control Actions", func(t *testing.T) {
		// Test invalid action
		invalidReq := map[string]interface{}{
			"player": "test-player",
			"action": "invalid-action",
		}
		invalidBody, _ := json.Marshal(invalidReq)

		req, _ := http.NewRequest("POST", "/api/player/control", bytes.NewBuffer(invalidBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.False(t, response.Success)
		assert.NotEmpty(t, response.Error)
	})

	t.Run("Nonexistent Worker Operations (graceful handling)", func(t *testing.T) {
		nonexistentPlayer := "nonexistent-player-12345"

		// Try to control nonexistent worker (system handles gracefully)
		controlReq := map[string]interface{}{
			"player": nonexistentPlayer,
			"action": "pause",
		}
		controlBody, _ := json.Marshal(controlReq)

		req, _ := http.NewRequest("POST", "/api/workers/control", bytes.NewBuffer(controlBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// System handles nonexistent workers gracefully
		assert.Equal(t, http.StatusOK, w.Code, "System should handle nonexistent workers gracefully")

		var response models.APIResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)

		// Try to delete nonexistent player (also handled gracefully)
		deleteReq := map[string]interface{}{
			"player": nonexistentPlayer,
		}
		deleteBody, _ := json.Marshal(deleteReq)

		req, _ = http.NewRequest("POST", "/api/player/delete", bytes.NewBuffer(deleteBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// System handles nonexistent players gracefully
		assert.Equal(t, http.StatusOK, w.Code, "System should handle nonexistent players gracefully")

		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)
	})
}

// TestRosterTogglesConcurrency tests concurrent roster toggle operations
func TestRosterTogglesConcurrency(t *testing.T) {
	// Skip if not integration test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup test configuration
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
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
	router.POST("/api/workers/control", h.ControlWorker)
	router.POST("/api/player/delete", h.DeletePlayer)

	t.Run("Concurrent Pause and Resume Operations", func(t *testing.T) {
		// Create multiple workers
		workerNames := []string{
			fmt.Sprintf("concurrent-test-1-%d", time.Now().Unix()),
			fmt.Sprintf("concurrent-test-2-%d", time.Now().Unix()),
			fmt.Sprintf("concurrent-test-3-%d", time.Now().Unix()),
		}

		// Start all workers
		for _, workerName := range workerNames {
			startReq := map[string]interface{}{
				"player":           workerName,
				"skills":           []string{"gather"},
				"fail_pct":         0.0,
				"speed_multiplier": 1.0,
				"workers":          1,
			}
			startBody, _ := json.Marshal(startReq)

			req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(startBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		}

		// Wait for workers to be ready
		time.Sleep(3 * time.Second)

		// Perform concurrent pause operations
		done := make(chan bool, len(workerNames))
		for _, workerName := range workerNames {
			go func(name string) {
				pauseReq := map[string]interface{}{
					"player": name,
					"action": "pause",
				}
				pauseBody, _ := json.Marshal(pauseReq)

				req, _ := http.NewRequest("POST", "/api/workers/control", bytes.NewBuffer(pauseBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code, "Failed to pause worker %s: %s", name, w.Body.String())
				done <- true
			}(workerName)
		}

		// Wait for all pause operations to complete
		for i := 0; i < len(workerNames); i++ {
			<-done
		}

		// Perform concurrent resume operations
		for _, workerName := range workerNames {
			go func(name string) {
				resumeReq := map[string]interface{}{
					"player": name,
					"action": "resume",
				}
				resumeBody, _ := json.Marshal(resumeReq)

				req, _ := http.NewRequest("POST", "/api/workers/control", bytes.NewBuffer(resumeBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code, "Failed to resume worker %s: %s", name, w.Body.String())
				done <- true
			}(workerName)
		}

		// Wait for all resume operations to complete
		for i := 0; i < len(workerNames); i++ {
			<-done
		}

		// Cleanup: delete all workers
		for _, workerName := range workerNames {
			deleteReq := map[string]interface{}{
				"player": workerName,
			}
			deleteBody, _ := json.Marshal(deleteReq)

			req, _ := http.NewRequest("POST", "/api/player/delete", bytes.NewBuffer(deleteBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		}
	})
}
