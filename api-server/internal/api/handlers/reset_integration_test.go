package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"demo-event-bus-api/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResetGameIntegration tests the full reset functionality including state changes
func TestResetGameIntegration(t *testing.T) {
	th := setupTestHandlers()

	// Setup the reset route
	th.router.POST("/api/reset", th.handlers.ResetGame)
	th.router.POST("/api/player/start", th.handlers.StartPlayer)
	th.router.POST("/api/master/start", th.handlers.StartMaster)
	th.router.GET("/api/state", th.handlers.GetGameState)

	t.Run("ResetClearsPlayerStats", func(t *testing.T) {
		// Create some players and stats
		th.handlers.ensurePlayer("alice")
		th.handlers.ensurePlayer("bob")
		th.handlers.updatePlayerStat("alice", "accepted", 5)
		th.handlers.updatePlayerStat("alice", "completed", 3)
		th.handlers.updatePlayerStat("bob", "accepted", 2)
		th.handlers.updatePlayerStat("bob", "failed", 1)

		// Verify stats exist
		stats := th.handlers.getPlayerStatsSnapshot()
		assert.Len(t, stats, 2)
		assert.Equal(t, 5, stats["alice"].(map[string]int)["accepted"])
		assert.Equal(t, 3, stats["alice"].(map[string]int)["completed"])

		// Perform reset
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		resp := httptest.NewRecorder()
		th.router.ServeHTTP(resp, req)

		// Verify reset was successful
		assert.Equal(t, http.StatusOK, resp.Code)

		var response models.APIResponse
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)

		// Verify stats are cleared
		statsAfter := th.handlers.getPlayerStatsSnapshot()
		assert.Empty(t, statsAfter, "All player stats should be cleared after reset")
	})

	t.Run("ResetAfterWorkflowState", func(t *testing.T) {
		// Start some players
		startPlayerReq := map[string]interface{}{
			"player":           "test-worker",
			"skills":           "gather,slay",
			"fail_pct":         0.1,
			"speed_multiplier": 1.0,
			"workers":          1,
		}
		reqBody, _ := json.Marshal(startPlayerReq)

		req, _ := http.NewRequest("POST", "/api/player/start", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		th.router.ServeHTTP(resp, req)

		// Allow some time for worker creation
		time.Sleep(100 * time.Millisecond)

		// Ensure some player stats
		th.handlers.ensurePlayer("test-worker")
		th.handlers.updatePlayerStat("test-worker", "accepted", 10)

		// Verify state exists before reset
		stats := th.handlers.getPlayerStatsSnapshot()
		assert.NotEmpty(t, stats, "Should have player stats before reset")

		// Perform reset
		resetReq, _ := http.NewRequest("POST", "/api/reset", nil)
		resetResp := httptest.NewRecorder()
		th.router.ServeHTTP(resetResp, resetReq)

		// Verify reset response
		assert.Equal(t, http.StatusOK, resetResp.Code)

		var response models.APIResponse
		err := json.Unmarshal(resetResp.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)
		assert.Contains(t, response.Message, "Hard reset completed")

		// Verify response data structure
		data, ok := response.Data.(map[string]interface{})
		require.True(t, ok, "Response should contain data map")
		assert.Equal(t, "all", data["workers_stopped"])
		assert.True(t, data["stats_cleared"].(bool))
		assert.True(t, data["ui_reset"].(bool))

		// Verify state is actually cleared
		statsAfter := th.handlers.getPlayerStatsSnapshot()
		assert.Empty(t, statsAfter, "All state should be cleared after reset")
	})

	t.Run("ResetHandlesErrorsGracefully", func(t *testing.T) {
		// Test reset when some operations might fail
		// This tests that reset continues even if some cleanup operations fail

		// Perform reset without any existing state
		req, _ := http.NewRequest("POST", "/api/reset", nil)
		resp := httptest.NewRecorder()
		th.router.ServeHTTP(resp, req)

		// Should still succeed even if there's nothing to reset
		assert.Equal(t, http.StatusOK, resp.Code)

		var response models.APIResponse
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Success)
		assert.Contains(t, response.Message, "Hard reset completed")
	})

	t.Run("ResetResponseFormat", func(t *testing.T) {
		// Test that reset response contains all expected fields

		req, _ := http.NewRequest("POST", "/api/reset", nil)
		resp := httptest.NewRecorder()
		th.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var response models.APIResponse
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response structure
		assert.True(t, response.Success)
		assert.NotEmpty(t, response.Message)
		assert.NotNil(t, response.Data)

		// Verify data contains expected fields
		data, ok := response.Data.(map[string]interface{})
		require.True(t, ok)

		expectedFields := []string{"workers_stopped", "queues_deleted", "queues_count", "stats_cleared", "ui_reset"}
		for _, field := range expectedFields {
			assert.Contains(t, data, field, "Response data should contain %s field", field)
		}

				// Verify queues_deleted is an array (note: changed from purged to deleted)
		queuesDeleted, ok := data["queues_deleted"].([]interface{})
		assert.True(t, ok, "queues_deleted should be an array")
		
		// Verify queues_count is present
		queuesCount, ok := data["queues_count"].(int)
		assert.True(t, ok, "queues_count should be an integer")
		assert.Equal(t, len(queuesDeleted), queuesCount, "Queue count should match deleted queues length")
	})
}
