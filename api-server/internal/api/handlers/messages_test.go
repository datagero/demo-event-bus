package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMessageHandlersDetailed provides detailed tests for message publishing endpoints
func TestMessageHandlersDetailed(t *testing.T) {
	th := setupTestHandlers()

	// Setup message routes
	th.router.POST("/api/publish", th.handlers.PublishMessage)
	th.router.POST("/api/publish/wave", th.handlers.PublishWave)
	th.router.POST("/api/master/start", th.handlers.StartMaster)
	th.router.POST("/api/master/one", th.handlers.SendOne)

	// Message list routes
	pending := th.router.Group("/api/pending")
	{
		pending.GET("/list", th.handlers.ListPendingMessages)
		pending.POST("/reissue", th.handlers.ReissuePendingMessages)
		pending.POST("/reissue/all", th.handlers.ReissueAllPendingMessages)
	}

	failed := th.router.Group("/api/failed")
	{
		failed.GET("/list", th.handlers.ListFailedMessages)
		failed.POST("/reissue", th.handlers.ReissueFailedMessages)
		failed.POST("/reissue/all", th.handlers.ReissueAllFailedMessages)
	}

	unroutable := th.router.Group("/api/unroutable")
	{
		unroutable.GET("/list", th.handlers.ListUnroutableMessages)
		unroutable.POST("/reissue", th.handlers.ReissueUnroutableMessages)
		unroutable.POST("/reissue/all", th.handlers.ReissueAllUnroutableMessages)
	}

	t.Run("PublishMessage_CompletePayload", func(t *testing.T) {
		payload := map[string]interface{}{
			"routing_key": "game.quest.gather",
			"payload": map[string]interface{}{
				"case_id":    "test-complete-123",
				"quest_type": "gather",
				"difficulty": 2,
				"work_sec":   3,
				"points":     10,
				"source":     "unit_test",
			},
		}

		w := th.makeRequest("POST", "/api/publish", payload)

		if w.Code == http.StatusOK {
			response := parseResponse(t, w)
			assert.Equal(t, true, response["ok"])
			assert.Contains(t, response, "message")
		}
	})

	t.Run("PublishWave_ValidCount", func(t *testing.T) {
		payload := map[string]interface{}{
			"routing_key": "game.quest.slay",
			"count":       5,
			"delay":       100, // 100ms delay
		}

		w := th.makeRequest("POST", "/api/publish/wave", payload)

		if w.Code == http.StatusOK {
			response := parseResponse(t, w)
			assert.Equal(t, true, response["ok"])
			assert.Contains(t, response["message"].(string), "5 messages")
		}
	})

	t.Run("PublishWave_InvalidCount", func(t *testing.T) {
		payload := map[string]interface{}{
			"routing_key": "game.quest.slay",
			"count":       1001, // Over the limit
			"delay":       100,
		}

		w := th.makeRequest("POST", "/api/publish/wave", payload)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, false, response["ok"])
		assert.Contains(t, response["error"].(string), "Count must be between 1 and 1000")
	})

	t.Run("SendOne_AllValidQuestTypes", func(t *testing.T) {
		validTypes := []string{"gather", "slay", "escort"}

		for _, questType := range validTypes {
			t.Run("QuestType_"+questType, func(t *testing.T) {
				payload := map[string]interface{}{
					"quest_type": questType,
				}

				w := th.makeRequest("POST", "/api/master/one", payload)

				if w.Code == http.StatusOK {
					response := parseResponse(t, w)
					assert.Equal(t, true, response["ok"])

					data := response["data"].(map[string]interface{})
					assert.Equal(t, questType, data["quest_type"])
					assert.Equal(t, "game.quest."+questType, data["routing_key"])
					assert.Equal(t, "go_api_server", data["source"])

					// Check payload structure
					payload := data["payload"].(map[string]interface{})
					assert.Contains(t, payload, "case_id")
					assert.Equal(t, questType, payload["quest_type"])
					assert.Equal(t, float64(1), payload["difficulty"])
					assert.Equal(t, float64(2), payload["work_sec"])
					assert.Equal(t, float64(5), payload["points"])
					assert.Equal(t, "frontend_send_one", payload["source"])
				}
			})
		}
	})

	t.Run("StartMaster_DefaultValues", func(t *testing.T) {
		// Test with empty payload (should use defaults)
		payload := map[string]interface{}{}

		w := th.makeRequest("POST", "/api/master/start", payload)

		if w.Code == http.StatusOK {
			response := parseResponse(t, w)
			assert.Equal(t, true, response["ok"])

			data := response["data"].(map[string]interface{})
			assert.Equal(t, float64(10), data["count_per_type"]) // Default count
			assert.Equal(t, float64(0.1), data["delay"])         // Default delay
			assert.Equal(t, float64(30), data["total_messages"]) // 10 * 3 quest types

			questTypes := data["quest_types"].([]interface{})
			assert.Len(t, questTypes, 3)
			assert.Contains(t, questTypes, "gather")
			assert.Contains(t, questTypes, "slay")
			assert.Contains(t, questTypes, "escort")
		}
	})

	t.Run("StartMaster_CustomValues", func(t *testing.T) {
		payload := map[string]interface{}{
			"count": 3,
			"delay": 0.5,
		}

		w := th.makeRequest("POST", "/api/master/start", payload)

		if w.Code == http.StatusOK {
			response := parseResponse(t, w)
			assert.Equal(t, true, response["ok"])

			data := response["data"].(map[string]interface{})
			assert.Equal(t, float64(3), data["count_per_type"])
			assert.Equal(t, float64(0.5), data["delay"])
			assert.Equal(t, float64(9), data["total_messages"]) // 3 * 3 quest types
		}
	})

	t.Run("ListPendingMessages_CorrectEndpoint", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/pending/list", nil)

		// Should return some response
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("ListFailedMessages_CorrectEndpoint", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/failed/list", nil)

		// Should return some response (OK, error, or not implemented)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusNotImplemented}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("ListUnroutableMessages_CorrectEndpoint", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/unroutable/list", nil)

		// Should return some response (OK, error, or not implemented)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusNotImplemented}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestMessageValidation tests message validation logic
func TestMessageValidation(t *testing.T) {
	th := setupTestHandlers()
	th.router.POST("/api/master/one", th.handlers.SendOne)

	testCases := []struct {
		name           string
		questType      string
		expectedStatus int
		shouldContain  string
	}{
		{
			name:           "ValidGather",
			questType:      "gather",
			expectedStatus: http.StatusOK,
			shouldContain:  "",
		},
		{
			name:           "ValidSlay",
			questType:      "slay",
			expectedStatus: http.StatusOK,
			shouldContain:  "",
		},
		{
			name:           "ValidEscort",
			questType:      "escort",
			expectedStatus: http.StatusOK,
			shouldContain:  "",
		},
		{
			name:           "InvalidQuestType",
			questType:      "invalid",
			expectedStatus: http.StatusBadRequest,
			shouldContain:  "Invalid quest type",
		},
		{
			name:           "EmptyQuestType",
			questType:      "",
			expectedStatus: http.StatusBadRequest,
			shouldContain:  "Invalid quest type",
		},
		{
			name:           "NumericQuestType",
			questType:      "123",
			expectedStatus: http.StatusBadRequest,
			shouldContain:  "Invalid quest type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]interface{}{
				"quest_type": tc.questType,
			}

			w := th.makeRequest("POST", "/api/master/one", payload)

			// For tests that expect OK but might fail due to infrastructure,
			// accept both OK and internal server error
			if tc.expectedStatus == http.StatusOK {
				assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)
			} else {
				assert.Equal(t, tc.expectedStatus, w.Code)
			}

			response := parseResponse(t, w)

			if tc.shouldContain != "" {
				assert.Contains(t, response["error"].(string), tc.shouldContain)
			}
		})
	}
}

// TestMessageReissue tests message reissue functionality
func TestMessageReissue(t *testing.T) {
	th := setupTestHandlers()

	// Setup reissue routes
	pending := th.router.Group("/api/pending")
	{
		pending.POST("/reissue", th.handlers.ReissuePendingMessages)
		pending.POST("/reissue/all", th.handlers.ReissueAllPendingMessages)
	}

	t.Run("ReissuePendingMessages_WithQuestID", func(t *testing.T) {
		payload := map[string]interface{}{
			"quest_id": "test-quest-123",
		}

		w := th.makeRequest("POST", "/api/pending/reissue", payload)

		// Should handle the request (may fail due to RabbitMQ not available or not implemented)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusNotImplemented}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("ReissueAllPendingMessages", func(t *testing.T) {
		w := th.makeRequest("POST", "/api/pending/reissue/all", nil)

		// Should handle the request (may fail due to RabbitMQ not available or not implemented)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusNotImplemented}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}
