package handlers_test

import (
	"bytes"

	"demo-event-bus-api/internal/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPIRequestValidation tests just the validation logic without external dependencies
func TestAPIRequestValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Worker Start Request Validation", func(t *testing.T) {
		// Create a minimal handler that just tests the binding
		router := gin.New()
		router.POST("/test/start", func(c *gin.Context) {
			var req struct {
				Player          string   `json:"player" binding:"required"`
				Skills          []string `json:"skills" binding:"required"`
				FailPct         float64  `json:"fail_pct"`
				SpeedMultiplier float64  `json:"speed_multiplier"`
				Workers         int      `json:"workers"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, models.APIResponse{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			// If validation passes, return success (don't call external services)
			c.JSON(http.StatusOK, models.APIResponse{
				Success: true,
				Message: "Validation passed",
			})
		})

		testCases := []struct {
			name         string
			requestBody  map[string]interface{}
			expectedCode int
			shouldPass   bool
		}{
			{
				name: "Valid Request",
				requestBody: map[string]interface{}{
					"player":           "alice-test",
					"skills":           []string{"gather", "slay"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusOK,
				shouldPass:   true,
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
					"player":           "alice-test",
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Wrong Field Name - worker_name instead of player",
				requestBody: map[string]interface{}{
					"worker_name":      "alice-test",
					"skills":           []string{"gather"},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Empty Skills Array (should pass validation)",
				requestBody: map[string]interface{}{
					"player":           "alice-test",
					"skills":           []string{},
					"fail_pct":         0.2,
					"speed_multiplier": 1.0,
				},
				expectedCode: http.StatusOK, // Empty array passes binding:"required"
				shouldPass:   true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				jsonBody, _ := json.Marshal(tc.requestBody)
				req, _ := http.NewRequest("POST", "/test/start", bytes.NewBuffer(jsonBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedCode, w.Code, "Expected HTTP status code")

				var response models.APIResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				if tc.shouldPass {
					assert.True(t, response.Success, "Valid requests should pass validation")
				} else {
					assert.False(t, response.Success, "Invalid requests should fail validation")
					assert.NotEmpty(t, response.Error, "Failed validation should have error message")
				}
			})
		}
	})

	t.Run("Worker Stop Request Validation", func(t *testing.T) {
		router := gin.New()
		router.POST("/test/stop", func(c *gin.Context) {
			var req struct {
				Player string `json:"player" binding:"required"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, models.APIResponse{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, models.APIResponse{
				Success: true,
				Message: "Validation passed",
			})
		})

		testCases := []struct {
			name         string
			requestBody  map[string]interface{}
			expectedCode int
			shouldPass   bool
		}{
			{
				name: "Valid Stop Request",
				requestBody: map[string]interface{}{
					"player": "alice-test",
				},
				expectedCode: http.StatusOK,
				shouldPass:   true,
			},
			{
				name:         "Missing Player Field",
				requestBody:  map[string]interface{}{},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
			{
				name: "Wrong Field Name",
				requestBody: map[string]interface{}{
					"worker_name": "alice-test",
				},
				expectedCode: http.StatusBadRequest,
				shouldPass:   false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				jsonBody, _ := json.Marshal(tc.requestBody)
				req, _ := http.NewRequest("POST", "/test/stop", bytes.NewBuffer(jsonBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedCode, w.Code)

				var response models.APIResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, tc.shouldPass, response.Success)
			})
		}
	})

	t.Run("Common API Patterns We Encountered", func(t *testing.T) {
		router := gin.New()
		router.POST("/test/worker", func(c *gin.Context) {
			var req struct {
				Player string   `json:"player" binding:"required"`
				Skills []string `json:"skills" binding:"required"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, models.APIResponse{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, models.APIResponse{
				Success: true,
				Data:    req,
				Message: "Request format is correct",
			})
		})

		// These were the actual problematic requests we encountered
		problematicRequests := []struct {
			name        string
			requestBody string
			description string
		}{
			{
				name:        "worker_name instead of player",
				requestBody: `{"worker_name":"alice","skills":["gather","slay"]}`,
				description: "This was the first error - wrong field name",
			},
			{
				name:        "missing player field entirely",
				requestBody: `{"skills":["gather"]}`,
				description: "Missing required player field",
			},
			{
				name:        "missing skills field",
				requestBody: `{"player":"alice"}`,
				description: "Missing required skills field",
			},
		}

		for _, tc := range problematicRequests {
			t.Run(tc.name, func(t *testing.T) {
				req, _ := http.NewRequest("POST", "/test/worker", bytes.NewBufferString(tc.requestBody))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusBadRequest, w.Code,
					"These problematic requests should be caught by validation")

				var response models.APIResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.False(t, response.Success)
				assert.NotEmpty(t, response.Error)

				t.Logf("Request: %s", tc.requestBody)
				t.Logf("Error: %s", response.Error)
				t.Logf("Description: %s", tc.description)
			})
		}

		// Test the correct format that should work
		t.Run("correct format that works", func(t *testing.T) {
			correctRequest := `{"player":"alice","skills":["gather","slay"],"fail_pct":0.2,"speed_multiplier":1.0}`

			req, _ := http.NewRequest("POST", "/test/worker", bytes.NewBufferString(correctRequest))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Correct format should pass validation")

			var response models.APIResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.True(t, response.Success)

			t.Logf("Correct request: %s", correctRequest)
		})
	})
}
