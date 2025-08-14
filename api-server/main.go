package main

import (
	"context"
	"demo-event-bus-api/internal/api"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/websocket"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// Command line flags
	var (
		port        = flag.String("port", "9000", "API server port")
		pythonURL   = flag.String("python", "http://localhost:8080", "Python service URL")
		workersURL  = flag.String("workers", "http://localhost:8001", "Go workers URL")
		rabbitMQURL = flag.String("rabbitmq", "amqp://guest:guest@localhost:5672/", "RabbitMQ URL")
	)
	flag.Parse()

	log.Printf("🚀 [API Server] Starting on port %s", *port)
	log.Printf("🐍 [API Server] Python service: %s", *pythonURL)
	log.Printf("⚡ [API Server] Workers service: %s", *workersURL)
	log.Printf("🐰 [API Server] RabbitMQ: %s", *rabbitMQURL)

	// Load configuration
	cfg := &config.Config{
		Port:        *port,
		PythonURL:   *pythonURL,
		WorkersURL:  *workersURL,
		RabbitMQURL: *rabbitMQURL,
	}

	// Initialize WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Set Gin mode
	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS middleware for development
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Setup API routes
	api.SetupRoutes(router, cfg, wsHub)

	// Create server
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Printf("✅ [API Server] Server ready on http://localhost:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ [API Server] Failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 [API Server] Shutting down...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("❌ [API Server] Forced shutdown: %v", err)
	} else {
		log.Println("✅ [API Server] Graceful shutdown completed")
	}
}
// Test comment for development workflow
