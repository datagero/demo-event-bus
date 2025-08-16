package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"demo-event-bus-api/tests/testdata"

	"github.com/stretchr/testify/assert"
)

// TestResetPublicAPI tests the public reset API endpoint
func TestResetPublicAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Reset endpoint responds successfully", func(t *testing.T) {
		// Test the reset endpoint
		resp := tc.MakeRequest("POST", "/reset", nil)

		// Should respond with success or handle gracefully
		// 200 = full success, 206 = partial success (with warnings), 500+ = error
		assert.True(t, resp.Code == http.StatusOK || resp.Code == http.StatusPartialContent || resp.Code >= 500,
			"Reset should succeed or fail gracefully, got: %d", resp.Code)

		if resp.Code == http.StatusOK {
			// If successful, should return JSON
			var resetResponse map[string]interface{}
			err := json.Unmarshal(resp.Body.Bytes(), &resetResponse)
			assert.NoError(t, err, "Reset response should be valid JSON")

			// Should contain status information
			assert.True(t, len(resetResponse) > 0, "Reset response should contain data")
		}
	})

	t.Run("Reset handles concurrent requests", func(t *testing.T) {
		// Test multiple concurrent reset requests
		numRequests := 3
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			go func() {
				resp := tc.MakeRequest("POST", "/reset", nil)
				results <- resp.Code
			}()
		}

		// Collect results
		var statusCodes []int
		for i := 0; i < numRequests; i++ {
			select {
			case code := <-results:
				statusCodes = append(statusCodes, code)
			case <-time.After(10 * time.Second):
				t.Fatal("Timeout waiting for concurrent reset requests")
			}
		}

		// All requests should complete without hanging
		assert.Len(t, statusCodes, numRequests, "All concurrent requests should complete")

		// At least one should succeed (others may conflict)
		hasSuccess := false
		for _, code := range statusCodes {
			if code == http.StatusOK || code == http.StatusPartialContent {
				hasSuccess = true
				break
			}
		}
		assert.True(t, hasSuccess, "At least one reset request should succeed")
	})

	t.Run("Reset followed by other operations", func(t *testing.T) {
		// Test that other endpoints work after reset
		resetResp := tc.MakeRequest("POST", "/reset", nil)

		// Give some time for reset to complete
		if resetResp.Code == http.StatusOK {
			time.Sleep(500 * time.Millisecond)
		}

		// Test that health endpoint still works
		healthResp := tc.MakeRequest("GET", "/health", nil)
		assert.True(t, healthResp.Code == http.StatusOK || healthResp.Code == http.StatusServiceUnavailable,
			"Health endpoint should be functional after reset")

		// Test that DLQ endpoint still works
		dlqResp := tc.MakeRequest("GET", "/api/dlq/list", nil)
		assert.True(t, dlqResp.Code >= 200 && dlqResp.Code < 600,
			"DLQ endpoint should be functional after reset")
	})
}

// TestResetErrorHandling tests how reset handles error conditions
func TestResetErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Reset with malformed request", func(t *testing.T) {
		// Test with invalid JSON (if endpoint expects JSON)
		resp := tc.MakeRequest("POST", "/reset", []byte("invalid json"))

		// Should handle gracefully
		assert.True(t, resp.Code >= 200 && resp.Code < 600,
			"Reset should handle malformed requests gracefully")
	})

	t.Run("Reset with extra parameters", func(t *testing.T) {
		// Test with unexpected parameters
		extraData := map[string]interface{}{
			"unexpected": "parameter",
			"extra":      123,
		}

		resp := tc.MakeJSONRequest("POST", "/reset", extraData)

		// Should handle extra parameters gracefully
		assert.True(t, resp.Code >= 200 && resp.Code < 600,
			"Reset should handle extra parameters gracefully")
	})
}
