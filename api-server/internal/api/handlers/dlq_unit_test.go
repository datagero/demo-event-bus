package handlers

import (
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/websocket"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDLQHelperFunctions tests the utility functions used by DLQ management
func TestDLQHelperFunctions(t *testing.T) {
	// Create minimal handlers instance for testing helper methods
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
		Port:        "9001", // Test port
		WorkersURL:  "http://localhost:8001",
		PythonURL:   "http://localhost:8000",
	}

	// Create mock clients (don't need real connections for unit tests)
	rabbitMQClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)
	workersClient := clients.NewWorkersClient(cfg.WorkersURL)
	pythonClient := clients.NewPythonClient(cfg.PythonURL)
	hub := websocket.NewHub()

	h := &Handlers{
		RabbitMQClient: rabbitMQClient,
		WorkersClient:  workersClient,
		PythonClient:   pythonClient,
		WSHub:          hub,
		Config:         cfg,
	}

	t.Run("isDLQQueue_Recognition", func(t *testing.T) {
		testCases := []struct {
			queueName string
			expected  bool
			reason    string
		}{
			{"game.dlq.failed.q", true, "Standard DLQ failed queue"},
			{"game.dlq.unroutable.q", true, "Standard DLQ unroutable queue"},
			{"game.dlq.expired.q", true, "Standard DLQ expired queue"},
			{"game.dlq.retry.q", true, "Standard DLQ retry queue"},
			{"game.dlq.custom.q", true, "Custom DLQ queue"},
			{"game.skill.gather.q", false, "Regular game queue"},
			{"game.quest.slay.q", false, "Regular quest queue"},
			{"other.dlq.failed.q", true, "DLQ queue with different namespace (still detected as DLQ)"},
			{"game.dlq", true, "DLQ exchange or queue name"},
			{"dlq.failed.q", true, "DLQ queue without game prefix (still detected as DLQ)"},
			{"", false, "Empty queue name"},
			{"game.dlq.new_category.q", true, "New DLQ category should be recognized"},
		}

		for _, tc := range testCases {
			result := h.isDLQQueue(tc.queueName)
			assert.Equal(t, tc.expected, result, "Queue '%s': %s", tc.queueName, tc.reason)
		}
	})

	t.Run("categorizeDLQMessage_Classification", func(t *testing.T) {
		testCases := []struct {
			message  map[string]interface{}
			expected string
			reason   string
		}{
			// Test messages with x-death headers (standard RabbitMQ DLQ)
			{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"headers": map[string]interface{}{
							"x-death": []interface{}{
								map[string]interface{}{
									"reason": "rejected",
								},
							},
						},
					},
				},
				"failed",
				"Message rejected by consumer",
			},
			{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"headers": map[string]interface{}{
							"x-death": []interface{}{
								map[string]interface{}{
									"reason": "expired",
								},
							},
						},
					},
				},
				"expired",
				"Message expired due to TTL",
			},
			{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"headers": map[string]interface{}{
							"x-death": []interface{}{
								map[string]interface{}{
									"reason": "maxlen",
								},
							},
						},
					},
				},
				"maxlength",
				"Message rejected due to queue max length",
			},
			// Test messages with custom failure_reason
			{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"headers": map[string]interface{}{
							"failure_reason": "processing_error",
						},
					},
				},
				"failed",
				"Custom failure reason",
			},
			// Test messages without clear death info
			{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"headers": map[string]interface{}{},
					},
				},
				"failed",
				"Message without death info defaults to failed",
			},
			// Test malformed message
			{
				map[string]interface{}{},
				"failed",
				"Malformed message defaults to failed",
			},
		}

		for _, tc := range testCases {
			result := h.categorizeDLQMessage(tc.message)
			assert.Equal(t, tc.expected, result, "Categorization: %s", tc.reason)
		}
	})

	t.Run("extractDeathInfo_Parsing", func(t *testing.T) {
		testCases := []struct {
			message  map[string]interface{}
			expected map[string]interface{}
			reason   string
		}{
			{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"headers": map[string]interface{}{
							"x-death": []interface{}{
								map[string]interface{}{
									"reason": "rejected",
									"count":  float64(1),
									"queue":  "game.skill.gather.q",
								},
							},
						},
					},
				},
				map[string]interface{}{
					"reason": "rejected",
					"count":  float64(1),
					"queue":  "game.skill.gather.q",
				},
				"Standard x-death header extraction",
			},
			{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"headers": map[string]interface{}{
							"failure_reason": "custom_error",
							"retry_count":    float64(3),
						},
					},
				},
				map[string]interface{}{
					"count":  0,
					"reason": "unknown",
					"queue":  "unknown",
				},
				"Custom headers without x-death (returns default death info)",
			},
			{
				map[string]interface{}{},
				map[string]interface{}{
					"count":  0,
					"reason": "unknown",
					"queue":  "unknown",
				},
				"Empty message should return default death info",
			},
		}

		for _, tc := range testCases {
			result := h.extractDeathInfo(tc.message)

			// Check that all expected keys exist and have correct values
			for key, expectedValue := range tc.expected {
				actualValue, exists := result[key]
				assert.True(t, exists, "Key '%s' should exist in death info for: %s", key, tc.reason)
				assert.Equal(t, expectedValue, actualValue, "Value for key '%s' should match for: %s", key, tc.reason)
			}
		}
	})

	t.Run("DLQ_Queue_Naming_Patterns", func(t *testing.T) {
		// Test that our DLQ recognition works with the actual queue names
		// that are created by setupDLQTopology
		expectedDLQQueues := []string{
			"game.dlq.failed.q",
			"game.dlq.unroutable.q",
			"game.dlq.expired.q",
			"game.dlq.retry.q",
		}

		for _, queueName := range expectedDLQQueues {
			assert.True(t, h.isDLQQueue(queueName), "Queue '%s' should be recognized as DLQ queue", queueName)
		}

		// Test that regular game queues are not recognized as DLQ
		regularQueues := []string{
			"game.skill.gather.q",
			"game.skill.slay.q",
			"game.skill.escort.q",
			"game.quest.gather.q",
			"game.quest.slay.q",
			"game.quest.escort.q",
		}

		for _, queueName := range regularQueues {
			assert.False(t, h.isDLQQueue(queueName), "Queue '%s' should NOT be recognized as DLQ queue", queueName)
		}
	})
}

// TestDLQTopologyValidation tests DLQ topology setup validation
func TestDLQTopologyValidation(t *testing.T) {
	// Create handlers instance
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
		Port:        "9001", // Test port
		WorkersURL:  "http://localhost:8001",
		PythonURL:   "http://localhost:8000",
	}

	rabbitMQClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)
	workersClient := clients.NewWorkersClient(cfg.WorkersURL)
	pythonClient := clients.NewPythonClient(cfg.PythonURL)
	hub := websocket.NewHub()

	h := &Handlers{
		RabbitMQClient: rabbitMQClient,
		WorkersClient:  workersClient,
		PythonClient:   pythonClient,
		WSHub:          hub,
		Config:         cfg,
	}

	t.Run("setupDLQTopology_Parameters", func(t *testing.T) {
		// Test setupDLQTopology with various parameters
		// Note: This is a unit test so we don't expect it to actually create queues
		// We're testing the parameter handling and return structure

		testCases := []struct {
			replayExchange    string
			retryTTL          int
			maxRetries        int
			backoffMultiplier float64
			expectError       bool
			reason            string
		}{
			{"game.dlq.replay", 5000, 3, 2.0, false, "Standard valid parameters"},
			{"custom.replay", 10000, 5, 1.5, false, "Custom valid parameters"},
			{"", 1000, 1, 1.0, false, "Edge case: minimal parameters"},
			{"game.dlq.replay", 0, 0, 0.0, false, "Edge case: zero values (may be handled)"},
		}

		for _, tc := range testCases {
			// We expect this to fail due to no real RabbitMQ connection in unit test
			// but we're testing that the function handles parameters correctly
			result, err := h.setupDLQTopology(tc.replayExchange, tc.retryTTL, tc.maxRetries, tc.backoffMultiplier)

			if tc.expectError {
				assert.Error(t, err, "Should error for: %s", tc.reason)
			} else {
				// In unit test environment, we expect an error due to no RabbitMQ
				// but the result structure should be predictable
				if err != nil {
					assert.Contains(t, err.Error(), "failed to create", "Error should be about creation failure for: %s", tc.reason)
				} else {
					// If somehow it succeeds, check result structure
					assert.NotNil(t, result, "Result should not be nil for: %s", tc.reason)
					assert.Contains(t, result, "exchanges_created", "Result should contain exchanges_created for: %s", tc.reason)
					assert.Contains(t, result, "queues_created", "Result should contain queues_created for: %s", tc.reason)
				}
			}
		}
	})
}
