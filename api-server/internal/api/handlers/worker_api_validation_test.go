package handlers

import (
	"bytes"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/models"
	"demo-event-bus-api/internal/websocket"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerAPIValidationUnit tests API request validation without external services
func TestWorkerAPIValidationUnit(t *testing.T) {
	cfg := &config.Config{
		WorkersURL: "http://localhost:8001",
	}

	wsHub := websocket.NewHub()
	go wsHub.Run()

	h := &Handlers{
		WorkersClient: clients.NewWorkersClient(cfg.WorkersURL),
		WSHub:         wsHub,
		Config:        cfg,
		// Note: Not setting RabbitMQClient to avoid nil pointer issues in unit tests
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/workers/start", h.StartWorker)
	router.POST("/api/workers/stop", h.StopWorker)

	t.Run("Start Worker Request Validation", func(t *testing.T) {
		testCases := []struct {
			name         string
			requestBody  map[string]interface{}
			expectedCode int
			shouldPass   bool
		}{
			{
				name: "Valid Request Format",
				requestBody: map[string]interface{}{
					"player":           "alice-validation",
					"skills":           []string{"gather", "slay"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusInternalServerError, // Will fail due to no workers service, but validates request format
				shouldPass:   false,                          // Expected to fail at service level, not validation
			},
			{
				name: "Missing Player Field",
				requestBody: map[string]interface{}{
					"skills":           []string{"gather"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Missing Skills Field",
				requestBody: map[string]interface{}{
					"player":           "alice-validation",
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Wrong Field Name (worker_name instead of player)",
				requestBody: map[string]interface{}{
					"worker_name":      "alice-validation",
					"skills":           []string{"gather"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Empty Skills Array",
				requestBody: map[string]interface{}{
					"player":           "alice-validation",
					"skills":           []string{},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				jsonBody, _ := json.Marshal(tc.requestBody)
				req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("Test case: %s", tc.name))

				var response models.APIResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				if tc.expectedCode == http.StatusBadRequest {
					assert.False(t, response.Success, "Invalid requests should fail validation")
					assert.NotEmpty(t, response.Error, "Validation errors should have error messages")
				}

				if tc.expectedCode == http.StatusInternalServerError {
					// This means validation passed but external service failed
					// In a real integration test, this would succeed
					assert.False(t, response.Success)
					// Error should be about service connectivity, not validation
					assert.NotContains(t, response.Error, "validation", "Should not be a validation error")
				}
			})
		}
	})

	t.Run("Stop Worker Request Validation", func(t *testing.T) {
		testCases := []struct {
			name         string
			requestBody  map[string]interface{}
			expectedCode int
			shouldPass   bool
		}{
			{
				name: "Valid Stop Request",
				requestBody: map[string]interface{}{
					"player": "alice-validation",
				},
				expectedCode: http.StatusInternalServerError, // Will fail due to no workers service
				shouldPass:   false,
			},
			{
				name:         "Missing Player Field",
				requestBody:  map[string]interface{}{},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Wrong Field Name (worker_name instead of player)",
				requestBody: map[string]interface{}{
					"worker_name": "alice-validation",
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Empty Player Name",
				requestBody: map[string]interface{}{
					"player": "",
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				jsonBody, _ := json.Marshal(tc.requestBody)
				req, _ := http.NewRequest("POST", "/api/workers/stop", bytes.NewBuffer(jsonBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("Test case: %s", tc.name))

				var response models.APIResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				if tc.expectedCode == http.StatusBadRequest {
					assert.False(t, response.Success, "Invalid requests should fail validation")
					assert.NotEmpty(t, response.Error, "Validation errors should have error messages")
				}
			})
		}
	})

	t.Run("Content-Type Validation", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"player": "alice-validation",
			"skills": []string{"gather"},
		}
		jsonBody, _ := json.Marshal(reqBody)

		// Test without Content-Type header
		req, _ := http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		// Intentionally not setting Content-Type
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should fail validation due to missing/wrong Content-Type
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should reject requests without proper Content-Type")

		// Test with wrong Content-Type
		req, _ = http.NewRequest("POST", "/api/workers/start", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "text/plain")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should reject requests with wrong Content-Type")
	})
}
