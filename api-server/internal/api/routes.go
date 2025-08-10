package api

import (
	"demo-event-bus-api/internal/api/handlers"
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/websocket"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *gin.Engine, cfg *config.Config, wsHub *websocket.Hub) {
	// Initialize clients
	pythonClient := clients.NewPythonClient(cfg.PythonURL)
	workersClient := clients.NewWorkersClient(cfg.WorkersURL)
	rabbitMQClient := clients.NewRabbitMQClient(cfg.RabbitMQURL)

	// Create handlers
	h := &handlers.Handlers{
		PythonClient:   pythonClient,
		WorkersClient:  workersClient,
		WSHub:          wsHub,
		Config:         cfg,
		RabbitMQClient: rabbitMQClient,
	}

	// WebSocket endpoint
	router.GET("/ws", websocket.HandleWebSocket(wsHub))

	// Health check
	router.GET("/health", h.Health)
	router.GET("/api/health", h.Health)

	// Static files (serve the frontend)
	router.Static("/static", "../legacy/python/app/web_static")
	router.GET("/", func(c *gin.Context) {
		c.File("../legacy/python/app/web_static/index.html")
	})

	// Workers webhook endpoint (needs to be before API group)
	router.POST("/api/go-workers/webhook/events", h.ReceiveWorkerEvents)

	// API routes group
	api := router.Group("/api")
	{
		// Game state and management
		api.GET("/state", h.GetGameState)
		api.POST("/reset", h.ResetGame)

		// Player/Worker management
		players := api.Group("/players")
		{
			players.POST("/quickstart", h.QuickstartPlayers)
			players.POST("/start", h.StartPlayer)
			players.POST("/delete", h.DeletePlayer)
			players.POST("/control", h.ControlPlayer)
		}

		// Worker management (Go workers)
		workers := api.Group("/workers")
		{
			workers.GET("/status", h.GetWorkersStatus)
			workers.POST("/start", h.StartWorker)
			workers.POST("/stop", h.StopWorker)
			workers.POST("/control", h.ControlWorker)
		}

		// Message and queue management
		api.POST("/publish", h.PublishMessage)
		api.POST("/publish/wave", h.PublishWave)
		api.POST("/master/start", h.StartMaster)
		api.POST("/master/one", h.SendOne)

		// Message lists
		pending := api.Group("/pending")
		{
			pending.GET("/list", h.ListPendingMessages)
			pending.POST("/reissue", h.ReissuePendingMessages)
			pending.POST("/reissue/all", h.ReissueAllPendingMessages)
		}

		failed := api.Group("/failed")
		{
			failed.GET("/list", h.ListFailedMessages)
			failed.POST("/reissue", h.ReissueFailedMessages)
			failed.POST("/reissue/all", h.ReissueAllFailedMessages)
		}

		dlq := api.Group("/dlq")
		{
			dlq.POST("/setup", h.SetupDLQ)
			dlq.GET("/list", h.ListDLQMessages)
			dlq.POST("/reissue", h.ReissueDLQMessages)
			dlq.POST("/reissue/all", h.ReissueAllDLQMessages)
		}

		unroutable := api.Group("/unroutable")
		{
			unroutable.GET("/list", h.ListUnroutableMessages)
			unroutable.POST("/reissue", h.ReissueUnroutableMessages)
			unroutable.POST("/reissue/all", h.ReissueAllUnroutableMessages)
		}

		// Chaos engineering
		chaos := api.Group("/chaos")
		{
			chaos.GET("/status", h.GetChaosStatus)
			chaos.POST("/arm", h.ArmChaos)
			chaos.POST("/disarm", h.DisarmChaos)
			chaos.POST("/config", h.SetChaosConfig)
		}

		// Scenarios
		scenario := api.Group("/scenario")
		{
			scenario.POST("/run", h.RunScenario)
		}

		// Card game
		cardgame := api.Group("/cardgame")
		{
			cardgame.GET("/enabled", h.IsCardGameEnabled)
			cardgame.GET("/status", h.GetCardGameStatus)
			cardgame.POST("/start", h.StartCardGame)
			cardgame.POST("/stop", h.StopCardGame)
		}

		// Broker and routing information
		broker := api.Group("/broker")
		{
			broker.GET("/routes", h.GetBrokerRoutes)
			broker.GET("/queues", h.GetBrokerQueues)
		}

		// Metrics and monitoring
		api.GET("/metrics", h.GetMetrics)
		api.GET("/player_stats", h.GetPlayerStats)

		// RabbitMQ direct endpoints (Go-native, educational)
		rabbitmq := api.Group("/rabbitmq")
		{
			rabbitmq.GET("/metrics", h.GetRabbitMQMetrics)
			rabbitmq.GET("/queues", h.GetRabbitMQQueues)
			rabbitmq.GET("/consumers", h.GetRabbitMQConsumers)
			rabbitmq.GET("/exchanges", h.GetRabbitMQExchanges)
			rabbitmq.GET("/messages/:queue", h.PeekQueueMessages)

			// Frontend compatibility routes
			derived := rabbitmq.Group("/derived")
			{
				derived.GET("/metrics", h.GetRabbitMQMetrics)       // Frontend expects /api/rabbitmq/derived/metrics
				derived.GET("/scoreboard", h.GetRabbitMQScoreboard) // Frontend expects /api/rabbitmq/derived/scoreboard
			}
		}

		// DLQ inspection endpoint
		dlq.GET("/inspect", h.InspectDLQ) // Frontend expects /api/dlq/inspect

		// WebSocket RabbitMQ endpoint (placeholder)
		router.GET("/ws/rabbitmq", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "WebSocket RabbitMQ not implemented",
				"message": "Use /ws for general WebSocket connection",
			})
		})
	}

	// Add a catch-all for debugging
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Route not found",
			"path":  c.Request.URL.Path,
		})
	})
}
