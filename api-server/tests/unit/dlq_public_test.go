package handlers_test

import (
	"net/http"
	"testing"

	"demo-event-bus-api/tests/testdata"

	"github.com/stretchr/testify/assert"
)

// TestDLQPublicAPI tests the public DLQ API endpoints
// This tests the public interface without accessing internal methods
func TestDLQPublicAPI(t *testing.T) {
	// Create test context with proper setup
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("DLQ list endpoint responds correctly", func(t *testing.T) {
		// Test the DLQ list endpoint
		resp := tc.MakeRequest("GET", "/api/dlq/list", nil)

		// Should respond with success (even if no DLQ messages)
		tc.AssertStatusCode(resp, http.StatusOK)

		// Response should be valid JSON
		assert.Contains(t, resp.Header().Get("Content-Type"), "application/json")
	})

	t.Run("DLQ endpoint handles missing services gracefully", func(t *testing.T) {
		// When RabbitMQ is not available, should still respond gracefully
		resp := tc.MakeRequest("GET", "/api/dlq/list", nil)

		// Should not panic or return 500 - may return empty list or error message
		assert.True(t, resp.Code == http.StatusOK || resp.Code == http.StatusServiceUnavailable)
	})
}

// TestDLQUtilityMethods tests the now-public DLQ utility methods
func TestDLQUtilityMethods(t *testing.T) {
	tc := testdata.NewTestContext(t)

	t.Run("IsDLQQueue method", func(t *testing.T) {
		// Test the public IsDLQQueue method
		testCases := []struct {
			queueName string
			expected  bool
		}{
			{"game.dlq.failed.q", true},
			{"game.dlq.expired.q", true},
			{"normal.queue", false},
			{"game.queue", false},
		}

		for _, testCase := range testCases {
			result := tc.GetHandlers().IsDLQQueue(testCase.queueName)
			assert.Equal(t, testCase.expected, result, "Queue: %s", testCase.queueName)
		}
	})
}
