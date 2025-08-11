package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Health handles health check requests with optional detailed diagnostics
func (h *Handlers) Health(c *gin.Context) {
	detailed := c.Query("detailed") == "true"

	services := make(map[string]interface{})

	if detailed {
		// Detailed health checks
		services["rabbitmq"] = h.checkRabbitMQHealth()
		services["workers"] = h.checkWorkersHealth()
		services["api"] = map[string]interface{}{
			"status": "healthy",
			"routes": h.getCriticalRoutes(),
		}
	} else {
		// Simple health checks
		services["rabbitmq"] = "up"

		// Check Workers service
		if _, err := h.WorkersClient.GetStatus(); err != nil {
			services["workers"] = "down"
		} else {
			services["workers"] = "up"
		}

		services["api"] = "up"
		services["websocket"] = "up"
	}

	status := "healthy"
	// Check if any service is down in simple mode
	if !detailed {
		for _, serviceStatus := range services {
			if serviceStatus == "down" {
				status = "degraded"
				break
			}
		}
	}

	response := models.HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Services:  services,
	}

	c.JSON(http.StatusOK, response)
}

// checkRabbitMQHealth tests RabbitMQ connectivity
func (h *Handlers) checkRabbitMQHealth() map[string]interface{} {
	status := map[string]interface{}{
		"status":     "healthy",
		"checked_at": time.Now().UTC(),
	}

	// Try to get basic metrics from RabbitMQ
	if metrics, err := h.RabbitMQClient.DeriveMetricsFromRabbitMQ(); err != nil {
		status["status"] = "unhealthy"
		status["error"] = err.Error()
	} else {
		status["queues"] = metrics["total_queues"]
		status["consumers"] = metrics["total_consumers"]
		status["pending_messages"] = metrics["total_pending"]
	}

	return status
}

// checkWorkersHealth tests Workers service connectivity
func (h *Handlers) checkWorkersHealth() map[string]interface{} {
	status := map[string]interface{}{
		"status":     "healthy",
		"checked_at": time.Now().UTC(),
	}

	// Try to get worker status
	if workerStatus, err := h.WorkersClient.GetStatus(); err != nil {
		status["status"] = "unhealthy"
		status["error"] = err.Error()
	} else {
		if workers, ok := workerStatus["workers"].([]interface{}); ok {
			status["active_workers"] = len(workers)
		}
	}

	return status
}

// getCriticalRoutes returns status of critical frontend routes
func (h *Handlers) getCriticalRoutes() map[string]string {
	return map[string]string{
		"player_recruitment": "/api/player/start",
		"quickstart":         "/api/players/quickstart",
		"send_one":           "/api/master/one",
		"quest_wave":         "/api/master/start",
		"metrics":            "/api/rabbitmq/derived/metrics",
		"scoreboard":         "/api/rabbitmq/derived/scoreboard",
	}
}
