package handlers

import (
	"bytes"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/websocket"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandlers contains the test setup
type TestHandlers struct {
	handlers *Handlers
	router   *gin.Engine
}

// setupTestHandlers creates a test environment with mocked dependencies
func setupTestHandlers() *TestHandlers {
	gin.SetMode(gin.TestMode)

	// Create test config with test ports to avoid conflicts
	cfg := &config.Config{
		Port:        "9001", // Use test port to avoid conflicts with running app
		PythonURL:   "http://localhost:8080",
		WorkersURL:  "http://localhost:8002", // Use test workers port
		RabbitMQURL: "amqp://guest:guest@localhost:5672/",
	}

	// Create mock clients (these will be mocked in individual tests)
	pythonClient := clients.NewPythonClient(cfg.PythonURL)
	workersClient := clients.NewWorkersClient(cfg.WorkersURL)
	rabbitMQClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)
	wsHub := websocket.NewHub()
	// Start the hub in a goroutine to prevent WebSocket channel blocking in tests
	go wsHub.Run()

	// Create handlers
	h := &Handlers{
		PythonClient:   pythonClient,
		WorkersClient:  workersClient,
		WSHub:          wsHub,
		Config:         cfg,
		RabbitMQClient: rabbitMQClient,
	}

	// Create router
	router := gin.New()

	return &TestHandlers{
		handlers: h,
		router:   router,
	}
}

// makeRequest is a helper to make HTTP requests to the test server
func (th *TestHandlers) makeRequest(method, url string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	th.router.ServeHTTP(w, req)

	return w
}

// parseResponse parses JSON response into a map
func parseResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to parse response JSON")
	return response
}

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	th := setupTestHandlers()
	th.router.GET("/health", th.handlers.Health)

	w := th.makeRequest("GET", "/health", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	response := parseResponse(t, w)
	// Health status can be "healthy" or "degraded" depending on service availability
	assert.Contains(t, []string{"healthy", "degraded"}, response["status"])
	assert.Contains(t, response, "timestamp")
	assert.Contains(t, response, "services")
}

// TestGameStateEndpoints tests game state related endpoints
func TestGameStateEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	th.router.GET("/api/state", th.handlers.GetGameState)
	th.router.POST("/api/reset", th.handlers.ResetGame)

	t.Run("GetGameState", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/state", nil)

		// Should return some response (even if Python service is not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("ResetGame", func(t *testing.T) {
		// First, create some test players to reset
		th.handlers.ensurePlayer("test-player-1")
		th.handlers.ensurePlayer("test-player-2")

		// Verify we have some player stats before reset
		stats := th.handlers.getPlayerStatsSnapshot()
		assert.NotEmpty(t, stats, "Should have player stats before reset")

		// Perform the reset
		w := th.makeRequest("POST", "/api/reset", nil)

		// Should now succeed
		assert.Equal(t, http.StatusOK, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, true, response["ok"])
		assert.Contains(t, response["message"].(string), "Hard reset completed")

		// Verify the response contains expected data
		data, ok := response["data"].(map[string]interface{})
		assert.True(t, ok, "Response should contain data map")
		assert.Equal(t, "all", data["workers_stopped"])
		assert.True(t, data["stats_cleared"].(bool))
		assert.True(t, data["ui_reset"].(bool))

		// Verify player stats are cleared
		statsAfter := th.handlers.getPlayerStatsSnapshot()
		assert.Empty(t, statsAfter, "Player stats should be empty after reset")
	})
}

// TestPlayerEndpoints tests player management endpoints
func TestPlayerEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	players := th.router.Group("/api/players")
	{
		players.POST("/quickstart", th.handlers.QuickstartPlayers)
		players.POST("/start", th.handlers.StartPlayer)
		players.POST("/delete", th.handlers.DeletePlayer)
		players.POST("/control", th.handlers.ControlPlayer)
	}

	// Compatibility routes for frontend
	th.router.POST("/api/player/start", th.handlers.StartPlayer)

	t.Run("QuickstartPlayers_ValidPreset", func(t *testing.T) {
		payload := map[string]interface{}{
			"preset": "alice_bob",
		}

		w := th.makeRequest("POST", "/api/players/quickstart", payload)

		// Should handle the request (may fail due to workers service not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("QuickstartPlayers_InvalidPreset", func(t *testing.T) {
		payload := map[string]interface{}{
			"preset": "invalid_preset",
		}

		w := th.makeRequest("POST", "/api/players/quickstart", payload)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, false, response["ok"])
		assert.Contains(t, response["error"].(string), "Invalid preset")
	})

	t.Run("StartPlayer_ValidRequest", func(t *testing.T) {
		payload := map[string]interface{}{
			"player":           "test_player",
			"skills":           "gather,slay",
			"fail_pct":         0.1,
			"speed_multiplier": 1.0,
			"workers":          1,
		}

		w := th.makeRequest("POST", "/api/players/start", payload)

		// Should handle the request (may fail due to workers service not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("StartPlayer_MissingRequiredFields", func(t *testing.T) {
		payload := map[string]interface{}{
			"skills": "gather",
			// Missing required "player" field
		}

		w := th.makeRequest("POST", "/api/players/start", payload)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, false, response["ok"])
	})

	t.Run("StartPlayer_CompatibilityRoute_SingularAPI", func(t *testing.T) {
		// Test the compatibility route /api/player/start (singular) that frontend uses
		payload := map[string]interface{}{
			"player":           "test_player_compat",
			"skills":           "gather,slay",
			"fail_pct":         0.1,
			"speed_multiplier": 1.0,
			"workers":          1,
		}

		w := th.makeRequest("POST", "/api/player/start", payload)

		// Should handle the request (may fail due to workers service not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestWorkerEndpoints tests worker management endpoints
func TestWorkerEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	workers := th.router.Group("/api/workers")
	{
		workers.GET("/status", th.handlers.GetWorkersStatus)
		workers.POST("/start", th.handlers.StartWorker)
		workers.POST("/stop", th.handlers.StopWorker)
		workers.POST("/control", th.handlers.ControlWorker)
	}

	t.Run("GetWorkersStatus", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/workers/status", nil)

		// Should handle the request (may fail due to workers service not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("StartWorker_ValidRequest", func(t *testing.T) {
		payload := map[string]interface{}{
			"player":           "test_worker",
			"skills":           []string{"gather"},
			"fail_pct":         0.1,
			"speed_multiplier": 1.0,
			"workers":          1,
		}

		w := th.makeRequest("POST", "/api/workers/start", payload)

		// Should handle the request (may fail due to workers service not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestMessageEndpoints tests message publishing endpoints
func TestMessageEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	th.router.POST("/api/publish", th.handlers.PublishMessage)
	th.router.POST("/api/publish/wave", th.handlers.PublishWave)
	th.router.POST("/api/master/start", th.handlers.StartMaster)
	th.router.POST("/api/master/one", th.handlers.SendOne)

	t.Run("PublishMessage_ValidRequest", func(t *testing.T) {
		payload := map[string]interface{}{
			"routing_key": "game.quest.gather",
			"payload": map[string]interface{}{
				"case_id":    "test-123",
				"quest_type": "gather",
				"points":     5,
			},
		}

		w := th.makeRequest("POST", "/api/publish", payload)

		// Should handle the request (may fail due to RabbitMQ not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("PublishMessage_MissingRoutingKey", func(t *testing.T) {
		payload := map[string]interface{}{
			"payload": map[string]interface{}{
				"case_id": "test-123",
			},
			// Missing required "routing_key" field
		}

		w := th.makeRequest("POST", "/api/publish", payload)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, false, response["ok"])
	})

	t.Run("SendOne_ValidQuestType", func(t *testing.T) {
		payload := map[string]interface{}{
			"quest_type": "gather",
		}

		w := th.makeRequest("POST", "/api/master/one", payload)

		// Should handle the request (may fail due to RabbitMQ not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("SendOne_InvalidQuestType", func(t *testing.T) {
		payload := map[string]interface{}{
			"quest_type": "invalid_type",
		}

		w := th.makeRequest("POST", "/api/master/one", payload)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, false, response["ok"])
		assert.Contains(t, response["error"].(string), "Invalid quest type")
	})

	t.Run("StartMaster_ValidRequest", func(t *testing.T) {
		payload := map[string]interface{}{
			"count": 5,
			"delay": 0.5,
		}

		w := th.makeRequest("POST", "/api/master/start", payload)

		// Should handle the request (may fail due to RabbitMQ not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestRabbitMQEndpoints tests RabbitMQ direct endpoints
func TestRabbitMQEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	rabbitmq := th.router.Group("/api/rabbitmq")
	{
		rabbitmq.GET("/metrics", th.handlers.GetRabbitMQMetrics)
		rabbitmq.GET("/queues", th.handlers.GetRabbitMQQueues)
		rabbitmq.GET("/consumers", th.handlers.GetRabbitMQConsumers)
		rabbitmq.GET("/exchanges", th.handlers.GetRabbitMQExchanges)

		// Frontend compatibility routes
		derived := rabbitmq.Group("/derived")
		{
			derived.GET("/metrics", th.handlers.GetRabbitMQMetrics)
			derived.GET("/scoreboard", th.handlers.GetRabbitMQScoreboard)
		}
	}

	t.Run("GetRabbitMQMetrics", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/metrics", nil)

		// Should handle the request (may fail due to RabbitMQ not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("GetRabbitMQScoreboard", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/rabbitmq/derived/scoreboard", nil)

		// Should handle the request (may fail due to RabbitMQ not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestChaosEndpoints tests chaos engineering endpoints
func TestChaosEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	chaos := th.router.Group("/api/chaos")
	{
		chaos.GET("/status", th.handlers.GetChaosStatus)
		chaos.POST("/arm", th.handlers.ArmChaos)
		chaos.POST("/disarm", th.handlers.DisarmChaos)
		chaos.POST("/config", th.handlers.SetChaosConfig)
	}

	t.Run("GetChaosStatus", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/chaos/status", nil)

		// Should handle the request (may fail due to workers service not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("ArmChaos_ValidRequest", func(t *testing.T) {
		payload := map[string]interface{}{
			"action":       "rmq_purge_queue",
			"target_queue": "game.skill.gather.q",
		}

		w := th.makeRequest("POST", "/api/chaos/arm", payload)

		// Should handle the request (may fail due to dependencies not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestDLQEndpoints tests Dead Letter Queue endpoints
func TestDLQEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	dlq := th.router.Group("/api/dlq")
	{
		dlq.POST("/setup", th.handlers.SetupDLQ)
		dlq.GET("/list", th.handlers.ListDLQMessages)
		dlq.GET("/inspect", th.handlers.InspectDLQ)
		dlq.POST("/reissue", th.handlers.ReissueDLQMessages)
		dlq.POST("/reissue/all", th.handlers.ReissueAllDLQMessages)
	}

	t.Run("SetupDLQ", func(t *testing.T) {
		w := th.makeRequest("POST", "/api/dlq/setup", nil)

		// Should handle the request (may fail due to RabbitMQ not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("InspectDLQ", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/dlq/inspect", nil)

		// Should handle the request (may fail due to RabbitMQ not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestScenarioEndpoints tests scenario endpoints
func TestScenarioEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	th.router.POST("/api/scenario/run", th.handlers.RunScenario)

	t.Run("RunScenario_ValidScenario", func(t *testing.T) {
		payload := map[string]interface{}{
			"scenario": "quest-wave",
			"params": map[string]interface{}{
				"wave_size": 3,
			},
		}

		w := th.makeRequest("POST", "/api/scenario/run", payload)

		// Should handle the request (may fail due to dependencies not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})

	t.Run("RunScenario_InvalidScenario", func(t *testing.T) {
		payload := map[string]interface{}{
			"scenario": "invalid-scenario",
		}

		w := th.makeRequest("POST", "/api/scenario/run", payload)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, false, response["ok"])
		assert.Contains(t, response["error"].(string), "Unknown scenario")
	})
}

// TestCardGameEndpoints tests card game endpoints
func TestCardGameEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	cardgame := th.router.Group("/api/cardgame")
	{
		cardgame.GET("/enabled", th.handlers.IsCardGameEnabled)
		cardgame.GET("/status", th.handlers.GetCardGameStatus)
		cardgame.POST("/start", th.handlers.StartCardGame)
		cardgame.POST("/stop", th.handlers.StopCardGame)
	}

	t.Run("IsCardGameEnabled", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/cardgame/enabled", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, true, response["ok"])

		data := response["data"].(map[string]interface{})
		assert.Equal(t, false, data["enabled"])
	})

	t.Run("GetCardGameStatus", func(t *testing.T) {
		w := th.makeRequest("GET", "/api/cardgame/status", nil)

		// Should handle the request (may fail due to Python service not available)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)

		response := parseResponse(t, w)
		assert.Contains(t, response, "ok")
	})
}

// TestWebhookEndpoints tests webhook endpoints
func TestWebhookEndpoints(t *testing.T) {
	th := setupTestHandlers()

	// Setup routes
	th.router.POST("/api/go-workers/webhook/events", th.handlers.ReceiveWorkerEvents)

	t.Run("ReceiveWorkerEvents_ValidEvent", func(t *testing.T) {
		payload := map[string]interface{}{
			"type":     "message_event",
			"event":    "ACCEPTED",
			"player":   "test_player",
			"quest_id": "test-123",
		}

		w := th.makeRequest("POST", "/api/go-workers/webhook/events", payload)

		assert.Equal(t, http.StatusOK, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, true, response["ok"])
		assert.Equal(t, "Event received", response["message"])
	})

	t.Run("ReceiveWorkerEvents_MissingType", func(t *testing.T) {
		payload := map[string]interface{}{
			"event":    "ACCEPTED",
			"player":   "test_player",
			"quest_id": "test-123",
			// Missing required "type" field
		}

		w := th.makeRequest("POST", "/api/go-workers/webhook/events", payload)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		response := parseResponse(t, w)
		assert.Equal(t, false, response["ok"])
		assert.Contains(t, response["error"].(string), "Missing or invalid event type")
	})
}

// TestNotFoundRoute tests the catch-all route for 404s
func TestNotFoundRoute(t *testing.T) {
	th := setupTestHandlers()

	// Setup catch-all route
	th.router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Route not found",
			"path":  c.Request.URL.Path,
		})
	})

	w := th.makeRequest("GET", "/api/nonexistent", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)

	response := parseResponse(t, w)
	assert.Equal(t, "Route not found", response["error"])
	assert.Equal(t, "/api/nonexistent", response["path"])
}
