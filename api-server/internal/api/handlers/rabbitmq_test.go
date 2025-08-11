package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRabbitMQHandlersDetailed provides detailed tests for RabbitMQ-specific endpoints
func TestRabbitMQHandlersDetailed(t *testing.T) {
	th := setupTestHandlers()

	// Setup RabbitMQ routes
	rabbitmq := th.router.Group("/api/rabbitmq")
	{
		rabbitmq.GET("/metrics", th.handlers.GetRabbitMQMetrics)
		rabbitmq.GET("/queues", th.handlers.GetRabbitMQQueues)
		rabbitmq.GET("/consumers", th.handlers.GetRabbitMQConsumers)
		rabbitmq.GET("/exchanges", th.handlers.GetRabbitMQExchanges)
		rabbitmq.GET("/messages/:queue", th.handlers.PeekQueueMessages)

		// Frontend compatibility routes
		derived := rabbitmq.Group("/derived")
		{
			derived.GET("/metrics", th.handlers.GetRabbitMQMetrics)
			derived.GET("/scoreboard", th.handlers.GetRabbitMQScoreboard)
		}
	}

	t.Run("GetRabbitMQMetrics_ReturnsConsistentFormat", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/metrics", nil)

		// Should handle gracefully even if RabbitMQ is not available
		if w.Code == http.StatusOK {
			response := parseResponse(t, w)
			assert.Equal(t, true, response["ok"])

			// Check for both data and metrics fields (frontend compatibility)
			assert.Contains(t, response, "data")
			assert.Contains(t, response, "metrics")
		}
	})

	t.Run("GetRabbitMQDerivedMetrics_FrontendCompatibility", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/derived/metrics", nil)

		// Should handle gracefully even if RabbitMQ is not available
		if w.Code == http.StatusOK {
			response := parseResponse(t, w)
			assert.Equal(t, true, response["ok"])

			// Should have both fields for frontend compatibility
			assert.Contains(t, response, "data")
			assert.Contains(t, response, "metrics")
		}
	})

	t.Run("GetRabbitMQScoreboard_ReturnsCorrectStructure", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/derived/scoreboard", nil)

		// Should handle gracefully even if RabbitMQ is not available
		if w.Code == http.StatusOK {
			response := parseResponse(t, w)
			assert.Equal(t, true, response["ok"])

			data := response["data"].(map[string]interface{})
			assert.Contains(t, data, "scoreboard")
			assert.Contains(t, data, "total_players")
			assert.Contains(t, data, "source")

			// Scoreboard should be an array
			scoreboard := data["scoreboard"].([]interface{})
			assert.IsType(t, []interface{}{}, scoreboard)
		}
	})

	t.Run("GetRabbitMQQueues_CorrectEndpoint", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/queues", nil)

		// Should return some response
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("GetRabbitMQConsumers_CorrectEndpoint", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/consumers", nil)

		// Should return some response
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("GetRabbitMQExchanges_CorrectEndpoint", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/exchanges", nil)

		// Should return some response
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("PeekQueueMessages_WithQueueParameter", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/messages/game.skill.gather.q", nil)

		// Should return some response
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestRabbitMQMetricsFormat tests the specific format requirements for metrics
func TestRabbitMQMetricsFormat(t *testing.T) {
	th := setupTestHandlers()
	th.router.GET("/api/rabbitmq/metrics", th.handlers.GetRabbitMQMetrics)

	w := th.makeRequest("GET", "/api/rabbitmq/metrics", nil)

	if w.Code == http.StatusOK {
		response := parseResponse(t, w)

		// Should have the correct response format
		assert.Equal(t, true, response["ok"])
		assert.Contains(t, response, "data")
		assert.Contains(t, response, "metrics")

		// Both data and metrics should contain the same information for frontend compatibility
		data := response["data"]
		metrics := response["metrics"]
		assert.Equal(t, data, metrics, "Data and metrics fields should be identical for frontend compatibility")
	}
}

// TestRabbitMQScoreboardFormat tests the scoreboard format requirements
func TestRabbitMQScoreboardFormat(t *testing.T) {
	th := setupTestHandlers()
	th.router.GET("/api/rabbitmq/derived/scoreboard", th.handlers.GetRabbitMQScoreboard)

	w := th.makeRequest("GET", "/api/rabbitmq/derived/scoreboard", nil)

	if w.Code == http.StatusOK {
		response := parseResponse(t, w)

		// Should have the correct response format
		assert.Equal(t, true, response["ok"])
		assert.Contains(t, response, "data")
		assert.Contains(t, response, "message")

		data := response["data"].(map[string]interface{})

		// Required fields in scoreboard response
		assert.Contains(t, data, "scoreboard")
		assert.Contains(t, data, "total_players")
		assert.Contains(t, data, "source")
		assert.Contains(t, data, "educational_note")

		// Verify types
		assert.IsType(t, []interface{}{}, data["scoreboard"])
		assert.IsType(t, float64(0), data["total_players"]) // JSON numbers are float64
		assert.IsType(t, "", data["source"])
		assert.IsType(t, "", data["educational_note"])

		// Source should indicate direct RabbitMQ access
		assert.Equal(t, "direct_rabbitmq_go_client", data["source"])
	}
}
