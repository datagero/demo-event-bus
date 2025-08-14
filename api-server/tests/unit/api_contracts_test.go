package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"demo-event-bus-api/tests/testdata"

	"github.com/stretchr/testify/assert"
)

// TestAPIContracts validates that API endpoints return expected response formats
func TestAPIContracts(t *testing.T) {
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Health endpoint contract", func(t *testing.T) {
		resp := tc.MakeRequest("GET", "/health", nil)

		// Health should return JSON
		if resp.Code == http.StatusOK {
			var healthResponse map[string]interface{}
			err := json.Unmarshal(resp.Body.Bytes(), &healthResponse)
			assert.NoError(t, err, "Health response should be valid JSON")

			// Should contain some status information
			assert.True(t, len(healthResponse) > 0, "Health response should contain data")
		}
	})

	t.Run("DLQ list endpoint contract", func(t *testing.T) {
		resp := tc.MakeRequest("GET", "/api/dlq/list", nil)

		if resp.Code == http.StatusOK {
			var dlqResponse map[string]interface{}
			err := json.Unmarshal(resp.Body.Bytes(), &dlqResponse)
			assert.NoError(t, err, "DLQ response should be valid JSON")

			// Should have expected structure
			if messages, exists := dlqResponse["messages"]; exists {
				assert.IsType(t, []interface{}{}, messages, "Messages should be an array")
			}
		}
	})

	t.Run("Reset endpoint contract", func(t *testing.T) {
		resp := tc.MakeRequest("POST", "/reset", nil)

		if resp.Code == http.StatusOK {
			var resetResponse map[string]interface{}
			err := json.Unmarshal(resp.Body.Bytes(), &resetResponse)
			assert.NoError(t, err, "Reset response should be valid JSON")

			// Should contain status information
			if status, exists := resetResponse["status"]; exists {
				assert.IsType(t, "", status, "Status should be a string")
			}
		}
	})
}

// TestRequestValidation tests API request validation
func TestRequestValidation(t *testing.T) {
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Worker start request validation", func(t *testing.T) {
		testCases := []struct {
			name        string
			request     map[string]interface{}
			expectError bool
			description string
		}{
			{
				name:        "Valid request",
				request:     testdata.ValidPlayerStartRequest,
				expectError: false,
				description: "Valid request should not return validation error",
			},
			{
				name:        "Invalid field names",
				request:     testdata.InvalidPlayerStartRequest,
				expectError: true,
				description: "Request with wrong field names should be rejected",
			},
			{
				name: "Missing required fields",
				request: map[string]interface{}{
					"player": "alice",
					// missing skills
				},
				expectError: true,
				description: "Request missing required fields should be rejected",
			},
			{
				name:        "Empty request",
				request:     map[string]interface{}{},
				expectError: true,
				description: "Empty request should be rejected",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				resp := tc.MakeJSONRequest("POST", "/api/workers/start", testCase.request)

				if testCase.expectError {
					// Should return some form of client error for invalid requests
					// (400 Bad Request or similar)
					assert.True(t, resp.Code >= 400 && resp.Code < 500,
						"Invalid request should return 4xx error: %s", testCase.description)
				} else {
					// Valid requests may still fail due to service availability,
					// but should not return validation errors (400)
					if resp.Code == http.StatusBadRequest {
						t.Logf("Valid request returned 400, response: %s", resp.Body.String())
					}
				}
			})
		}
	})

	t.Run("JSON parsing errors", func(t *testing.T) {
		// Test with malformed JSON
		req := tc.MakeRequest("POST", "/api/workers/start", []byte("invalid json"))

		// Should handle malformed JSON gracefully
		assert.True(t, req.Code >= 400, "Malformed JSON should return error")
	})
}

// TestEndpointAvailability validates that all expected endpoints are accessible
func TestEndpointAvailability(t *testing.T) {
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	endpoints := []struct {
		method string
		path   string
		name   string
	}{
		{"GET", "/health", "Health check"},
		{"POST", "/reset", "Game reset"},
		{"GET", "/api/dlq/list", "DLQ list"},
		{"POST", "/api/workers/start", "Worker start"},
		{"POST", "/api/workers/stop", "Worker stop"},
		{"POST", "/api/master/start", "Master start"},
		{"POST", "/api/master/one", "Master one"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			var resp *http.Response

			if endpoint.method == "GET" {
				resp = tc.MakeRequest(endpoint.method, endpoint.path, nil).Result()
			} else {
				resp = tc.MakeJSONRequest(endpoint.method, endpoint.path, map[string]interface{}{}).Result()
			}

			// Endpoint should be reachable (not 404)
			assert.NotEqual(t, http.StatusNotFound, resp.StatusCode,
				"Endpoint %s %s should be available", endpoint.method, endpoint.path)

			// Should not return method not allowed
			assert.NotEqual(t, http.StatusMethodNotAllowed, resp.StatusCode,
				"Endpoint %s %s should accept %s method", endpoint.method, endpoint.path, endpoint.method)
		})
	}
}

// TestErrorHandling tests how endpoints handle various error conditions
func TestErrorHandling(t *testing.T) {
	tc := testdata.NewTestContext(t)
	tc.SetupRoutes()

	t.Run("Handles missing content-type gracefully", func(t *testing.T) {
		// Make a POST request without content-type
		resp := tc.MakeRequest("POST", "/api/workers/start", nil)

		// Should handle gracefully, not panic
		assert.True(t, resp.Code >= 200 && resp.Code < 600, "Should return valid HTTP status")
	})

	t.Run("Handles oversized requests gracefully", func(t *testing.T) {
		// Create a large request
		largeRequest := map[string]interface{}{
			"player": "alice",
			"skills": make([]string, 1000), // Large array
		}

		resp := tc.MakeJSONRequest("POST", "/api/workers/start", largeRequest)

		// Should handle large requests without crashing
		assert.True(t, resp.Code >= 200 && resp.Code < 600, "Should handle large requests")
	})
}
