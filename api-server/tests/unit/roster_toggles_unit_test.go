package handlers_test

import (
	"bytes"
	"demo-event-bus-api/internal/api/handlers"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/models"
	"demo-event-bus-api/internal/websocket"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestHandlers creates handlers for testing with minimal external dependencies
func createTestHandlers(t *testing.T) (*handlers.Handlers, *gin.Engine) {
	gin.SetMode(gin.TestMode)

	// Create test configuration
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
		WorkersURL:  "http://localhost:8001",
	}

	// Create test clients
	rabbitClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)
	workersClient := clients.NewWorkersClient(cfg.WorkersURL)
	wsHub := websocket.NewHub()

	// Create handlers
	h := &handlers.Handlers{
		RabbitMQClient: rabbitClient,
		WorkersClient:  workersClient,
		WSHub:          wsHub,
		Config:         cfg,
	}

	// Create router
	router := gin.New()
	router.Use(gin.Recovery())

	return h, router
}

// TestPlayerControlValidation tests input validation for ControlPlayer handler
func TestPlayerControlValidation(t *testing.T) {
	h, router := createTestHandlers(t)
	router.POST("/api/player/control", h.ControlPlayer)

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
		expectedError  bool
	}{
		{
			name: "Missing Player",
			payload: map[string]interface{}{
				"action": "pause",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Missing Action",
			payload: map[string]interface{}{
				"player": "alice-test",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Empty Player Name",
			payload: map[string]interface{}{
				"player": "",
				"action": "pause",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Empty Action",
			payload: map[string]interface{}{
				"player": "alice-test",
				"action": "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Valid Request Structure (should succeed with workers service running)",
			payload: map[string]interface{}{
				"player": "alice-test",
				"action": "pause",
			},
			expectedStatus: http.StatusOK, // Workers service is running
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/player/control", bytes.NewBuffer(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response models.APIResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectedError {
				assert.False(t, response.Success)
				assert.NotEmpty(t, response.Error)
			} else {
				assert.True(t, response.Success)
				assert.Empty(t, response.Error)
				assert.NotEmpty(t, response.Message)
			}
		})
	}
}

// TestWorkerControlValidation tests input validation for ControlWorker handler
func TestWorkerControlValidation(t *testing.T) {
	h, router := createTestHandlers(t)
	router.POST("/api/workers/control", h.ControlWorker)

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
		expectedError  bool
	}{
		{
			name: "Missing Player",
			payload: map[string]interface{}{
				"action": "pause",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Missing Action",
			payload: map[string]interface{}{
				"player": "worker-test",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Empty Player Name",
			payload: map[string]interface{}{
				"player": "",
				"action": "pause",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Empty Action",
			payload: map[string]interface{}{
				"player": "worker-test",
				"action": "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Valid Request Structure (should succeed with workers service running)",
			payload: map[string]interface{}{
				"player": "worker-test",
				"action": "pause",
			},
			expectedStatus: http.StatusOK, // Workers service is running
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/workers/control", bytes.NewBuffer(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response models.APIResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectedError {
				assert.False(t, response.Success)
				assert.NotEmpty(t, response.Error)
			} else {
				assert.True(t, response.Success)
				assert.Empty(t, response.Error)
				assert.NotEmpty(t, response.Message)
			}
		})
	}
}

// TestPlayerDeleteValidation tests input validation for DeletePlayer handler
func TestPlayerDeleteValidation(t *testing.T) {
	h, router := createTestHandlers(t)
	router.POST("/api/player/delete", h.DeletePlayer)

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
		expectedError  bool
	}{
		{
			name:           "Missing Player",
			payload:        map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Empty Player Name",
			payload: map[string]interface{}{
				"player": "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Valid Request Structure (should succeed with workers service running)",
			payload: map[string]interface{}{
				"player": "alice-test",
			},
			expectedStatus: http.StatusOK, // Workers service is running
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/player/delete", bytes.NewBuffer(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response models.APIResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectedError {
				assert.False(t, response.Success)
				assert.NotEmpty(t, response.Error)
			} else {
				assert.True(t, response.Success)
				assert.Empty(t, response.Error)
				assert.NotEmpty(t, response.Message)
			}
		})
	}
}

// TestWorkerStopValidation tests input validation for StopWorker handler
func TestWorkerStopValidation(t *testing.T) {
	h, router := createTestHandlers(t)
	router.POST("/api/workers/stop", h.StopWorker)

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
		expectedError  bool
	}{
		{
			name:           "Missing Player",
			payload:        map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Empty Player Name",
			payload: map[string]interface{}{
				"player": "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name: "Valid Request Structure (should succeed with workers service running)",
			payload: map[string]interface{}{
				"player": "worker-test",
			},
			expectedStatus: http.StatusOK, // Workers service is running
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/workers/stop", bytes.NewBuffer(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response models.APIResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectedError {
				assert.False(t, response.Success)
				assert.NotEmpty(t, response.Error)
			} else {
				assert.True(t, response.Success)
				assert.Empty(t, response.Error)
				assert.NotEmpty(t, response.Message)
			}
		})
	}
}

// TestMalformedJSONRequests tests malformed JSON handling across all roster toggle endpoints
func TestMalformedJSONRequests(t *testing.T) {
	endpoints := []struct {
		name        string
		method      string
		url         string
		setupRouter func(*handlers.Handlers, *gin.Engine)
	}{
		{
			name:   "Player Control",
			method: "POST",
			url:    "/api/player/control",
			setupRouter: func(h *handlers.Handlers, r *gin.Engine) {
				r.POST("/api/player/control", h.ControlPlayer)
			},
		},
		{
			name:   "Worker Control",
			method: "POST",
			url:    "/api/workers/control",
			setupRouter: func(h *handlers.Handlers, r *gin.Engine) {
				r.POST("/api/workers/control", h.ControlWorker)
			},
		},
		{
			name:   "Player Delete",
			method: "POST",
			url:    "/api/player/delete",
			setupRouter: func(h *handlers.Handlers, r *gin.Engine) {
				r.POST("/api/player/delete", h.DeletePlayer)
			},
		},
		{
			name:   "Worker Stop",
			method: "POST",
			url:    "/api/workers/stop",
			setupRouter: func(h *handlers.Handlers, r *gin.Engine) {
				r.POST("/api/workers/stop", h.StopWorker)
			},
		},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name+" with malformed JSON", func(t *testing.T) {
			h, router := createTestHandlers(t)
			endpoint.setupRouter(h, router)

			req, err := http.NewRequest(endpoint.method, endpoint.url, bytes.NewBuffer([]byte("invalid json")))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}
