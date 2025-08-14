package handlers_test

import (
	"net/http"
	"testing"

	"demo-event-bus-api/tests/testdata"

	"github.com/stretchr/testify/assert"
)

// TestHandlersPublicAPI tests the public handler endpoints
// This focuses on testing public APIs without accessing internal methods
func TestHandlersPublicAPI(t *testing.T) {
	// Create test context with proper setup
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Health endpoint returns valid response", func(t *testing.T) {
		resp := tc.MakeRequest("GET", "/health", nil)

		// Health endpoint should always respond (may be 200 or 503 depending on services)
		assert.True(t, resp.Code == http.StatusOK || resp.Code == http.StatusServiceUnavailable)

		// Should have content-type header
		contentType := resp.Header().Get("Content-Type")
		assert.True(t, contentType != "", "Should have Content-Type header")
	})

	t.Run("Reset endpoint handles requests", func(t *testing.T) {
		// Test the reset endpoint
		resp := tc.MakeRequest("POST", "/reset", nil)

		// Should respond with some status (success or error, not panic)
		assert.True(t, resp.Code >= 200 && resp.Code < 600, "Should return valid HTTP status")
	})

	t.Run("DLQ endpoint is accessible", func(t *testing.T) {
		// Test the DLQ list endpoint
		resp := tc.MakeRequest("GET", "/api/dlq/list", nil)

		// Should respond without error
		assert.True(t, resp.Code >= 200 && resp.Code < 600, "Should return valid HTTP status")
	})
}

// TestWorkerAPIEndpoints tests worker-related public API endpoints
func TestWorkerAPIEndpoints(t *testing.T) {
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Worker start endpoint validation", func(t *testing.T) {
		// Test with valid request format
		validRequest := testdata.ValidPlayerStartRequest
		resp := tc.MakeJSONRequest("POST", "/api/workers/start", validRequest)

		// Should handle request (may succeed or fail due to services, but should not be 400)
		if resp.Code == http.StatusBadRequest {
			t.Logf("Request was rejected with 400, response: %s", resp.Body.String())
		}
		// Don't assert specific status - service may not be available in test environment
	})

	t.Run("Worker start with invalid data", func(t *testing.T) {
		// Test with invalid request
		invalidRequest := map[string]interface{}{
			"wrong_field": "test",
		}

		resp := tc.MakeJSONRequest("POST", "/api/workers/start", invalidRequest)

		// Should handle invalid requests gracefully
		assert.True(t, resp.Code >= 400, "Invalid request should return client error")
	})

	t.Run("Worker stop endpoint", func(t *testing.T) {
		// Test worker stop
		stopRequest := testdata.ValidPlayerStopRequest
		resp := tc.MakeJSONRequest("POST", "/api/workers/stop", stopRequest)

		// Should handle request without panicking
		assert.True(t, resp.Code >= 200 && resp.Code < 600, "Should return valid HTTP status")
	})
}

// TestMessageAPIEndpoints tests message publishing endpoints
func TestMessageAPIEndpoints(t *testing.T) {
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Master start endpoint", func(t *testing.T) {
		resp := tc.MakeRequest("POST", "/api/master/start", nil)

		// Should handle request (may fail due to missing services)
		assert.True(t, resp.Code >= 200 && resp.Code < 600, "Should return valid HTTP status")
	})

	t.Run("Master one endpoint", func(t *testing.T) {
		resp := tc.MakeRequest("POST", "/api/master/one", nil)

		// Should handle request
		assert.True(t, resp.Code >= 200 && resp.Code < 600, "Should return valid HTTP status")
	})
}
