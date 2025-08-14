package testdata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"demo-event-bus-api/internal/api/handlers"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContext provides common testing utilities and setup
type TestContext struct {
	t        *testing.T
	router   *gin.Engine
	handlers *handlers.Handlers
}

// NewTestContext creates a new test context with common setup
func NewTestContext(t *testing.T) *TestContext {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create test configuration
	cfg := &config.Config{
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
		Port:        "8000",
		PythonURL:   "http://localhost:8001",
		WorkersURL:  "http://localhost:8002",
	}

	// Create test clients
	rabbitClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)

	// Create WebSocket hub (but don't start it)
	wsHub := websocket.NewHub()

	// Create all clients for handlers
	pythonClient := clients.NewPythonClient(cfg.PythonURL)
	workersClient := clients.NewWorkersClient(cfg.WorkersURL)

	// Create handlers - manually instantiate since there's no constructor function
	h := &handlers.Handlers{
		RabbitMQClient: rabbitClient,
		WorkersClient:  workersClient,
		PythonClient:   pythonClient,
		WSHub:          wsHub,
		Config:         cfg,
	}

	// Create router
	router := gin.New()

	// Add basic middleware for testing
	router.Use(gin.Recovery())

	return &TestContext{
		t:        t,
		router:   router,
		handlers: h,
	}
}

// GetHandlers returns the handlers instance for direct access to public methods
func (tc *TestContext) GetHandlers() *handlers.Handlers {
	return tc.handlers
}

// SetupRoutes configures the router with API routes
func (tc *TestContext) SetupRoutes() {
	// Health endpoints
	tc.router.GET("/health", tc.handlers.Health)

	// Game endpoints
	tc.router.POST("/reset", tc.handlers.ResetGame)

	// Worker endpoints
	tc.router.POST("/api/workers/start", tc.handlers.StartWorker)
	tc.router.POST("/api/workers/stop", tc.handlers.StopWorker)

	// Master/Quest endpoints
	tc.router.POST("/api/master/one", tc.handlers.SendOne)
	tc.router.POST("/api/master/start", tc.handlers.StartMaster)

	// DLQ endpoints
	tc.router.GET("/api/dlq/list", tc.handlers.ListDLQMessages)
}

// MakeRequest makes an HTTP request and returns the response
func (tc *TestContext) MakeRequest(method, url string, body interface{}) *httptest.ResponseRecorder {
	var req *http.Request
	var err error

	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(tc.t, err)
		req, err = http.NewRequest(method, url, bytes.NewBuffer(jsonBody))
		require.NoError(tc.t, err)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
		require.NoError(tc.t, err)
	}

	w := httptest.NewRecorder()
	tc.router.ServeHTTP(w, req)

	return w
}

// MakeJSONRequest is a convenience method for JSON requests
func (tc *TestContext) MakeJSONRequest(method, url string, body interface{}) *httptest.ResponseRecorder {
	return tc.MakeRequest(method, url, body)
}

// AssertJSONResponse asserts the response has the expected status and JSON content
func (tc *TestContext) AssertJSONResponse(w *httptest.ResponseRecorder, expectedStatus int, expectedBody interface{}) {
	assert.Equal(tc.t, expectedStatus, w.Code)

	if expectedBody != nil {
		var actualBody interface{}
		err := json.Unmarshal(w.Body.Bytes(), &actualBody)
		require.NoError(tc.t, err)

		expectedJSON, err := json.Marshal(expectedBody)
		require.NoError(tc.t, err)

		var expectedParsed interface{}
		err = json.Unmarshal(expectedJSON, &expectedParsed)
		require.NoError(tc.t, err)

		assert.Equal(tc.t, expectedParsed, actualBody)
	}
}

// AssertStatusCode asserts the response has the expected status code
func (tc *TestContext) AssertStatusCode(w *httptest.ResponseRecorder, expectedStatus int) {
	assert.Equal(tc.t, expectedStatus, w.Code)
}

// WaitForCondition waits for a condition to be true with timeout
func (tc *TestContext) WaitForCondition(condition func() bool, timeout time.Duration, message string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	tc.t.Fatalf("Timeout waiting for condition: %s", message)
}

// ServiceHealthCheck checks if external services are available
func (tc *TestContext) ServiceHealthCheck() (available []string, missing []string) {
	// This is a simple check - in real tests you'd check RabbitMQ, etc.
	// For now, just return mock data
	available = []string{"mock-service"}
	missing = []string{}

	return available, missing
}

// IsIntegrationTest returns true if running integration tests
func IsIntegrationTest(t *testing.T) bool {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
		return false
	}
	return true
}

// SkipIfNoServices skips the test if required services are not available
func (tc *TestContext) SkipIfNoServices(services ...string) {
	available, missing := tc.ServiceHealthCheck()

	for _, required := range services {
		found := false
		for _, avail := range available {
			if avail == required {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		tc.t.Skipf("Skipping test - missing required services: %v", missing)
	}
}

// CleanupTest performs test cleanup
func (tc *TestContext) CleanupTest() {
	// Cleanup logic - close connections, reset state, etc.
	// For now, this is a placeholder
}

// MockRabbitMQHelper provides mocked RabbitMQ operations for testing
type MockRabbitMQHelper struct {
	tc *TestContext
}

// NewMockRabbitMQHelper creates a new mock RabbitMQ helper
func NewMockRabbitMQHelper(tc *TestContext) *MockRabbitMQHelper {
	return &MockRabbitMQHelper{tc: tc}
}

// Reset mocks a RabbitMQ reset operation
func (m *MockRabbitMQHelper) Reset() error {
	// Mock implementation
	return nil
}

// GetQueueCount mocks getting queue message count
func (m *MockRabbitMQHelper) GetQueueCount(queueName string) (int, error) {
	// Mock implementation - return 0 for empty queues
	return 0, nil
}

// PeekQueue mocks peeking at queue messages
func (m *MockRabbitMQHelper) PeekQueue(queueName string, count int) ([]interface{}, error) {
	// Mock implementation - return empty slice
	return []interface{}{}, nil
}

// Validation helpers
func ValidatePlayerRequest(data map[string]interface{}) error {
	if _, ok := data["player"]; !ok {
		return fmt.Errorf("missing required field: player")
	}
	if _, ok := data["skills"]; !ok {
		return fmt.Errorf("missing required field: skills")
	}
	return nil
}

func ValidateQuickstartRequest(data map[string]interface{}) error {
	if _, ok := data["player_count"]; !ok {
		return fmt.Errorf("missing required field: player_count")
	}
	if _, ok := data["skills"]; !ok {
		return fmt.Errorf("missing required field: skills")
	}
	return nil
}
