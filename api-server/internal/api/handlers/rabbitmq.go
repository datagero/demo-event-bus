package handlers

import (
	"demo-event-bus-api/internal/models"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetRabbitMQMetrics retrieves metrics directly from RabbitMQ Management API
func (h *Handlers) GetRabbitMQMetrics(c *gin.Context) {
	metrics, err := h.RabbitMQClient.DeriveMetricsFromRabbitMQ()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Return metrics in both 'data' and 'metrics' fields for frontend compatibility
	response := models.APIResponse{
		Success: true,
		Data:    metrics,
		Message: "Metrics derived directly from RabbitMQ Management API",
	}

	// Add metrics field for frontend compatibility (frontend expects metricsResult.metrics)
	responseMap := map[string]interface{}{
		"ok":      response.Success,
		"data":    response.Data,
		"metrics": metrics, // Frontend expects this field
		"message": response.Message,
	}

	c.JSON(http.StatusOK, responseMap)
}

// GetRabbitMQQueues retrieves queue information directly from RabbitMQ
func (h *Handlers) GetRabbitMQQueues(c *gin.Context) {
	queues, err := h.RabbitMQClient.GetQueuesFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    queues,
		Message: "Queue information retrieved directly from RabbitMQ",
	})
}

// GetRabbitMQConsumers retrieves consumer information directly from RabbitMQ
func (h *Handlers) GetRabbitMQConsumers(c *gin.Context) {
	consumers, err := h.RabbitMQClient.GetConsumersFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    consumers,
		Message: "Consumer information retrieved directly from RabbitMQ",
	})
}

// GetRabbitMQExchanges retrieves exchange information directly from RabbitMQ
func (h *Handlers) GetRabbitMQExchanges(c *gin.Context) {
	exchanges, err := h.RabbitMQClient.GetExchangesFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    exchanges,
		Message: "Exchange information retrieved directly from RabbitMQ",
	})
}

// PeekQueueMessages retrieves messages from a specific queue
func (h *Handlers) PeekQueueMessages(c *gin.Context) {
	queueName := c.Param("queue")
	if queueName == "" {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Queue name is required",
		})
		return
	}

	// Get count from query parameter, default to 5
	countStr := c.DefaultQuery("count", "5")
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		count = 5
	}

	messages, err := h.RabbitMQClient.PeekQueueMessages(queueName, count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"queue_name": queueName,
			"messages":   messages,
			"count":      len(messages),
		},
		Message: "Messages retrieved directly from RabbitMQ queue",
	})
}

// GetRabbitMQScoreboard provides player scoreboard derived from RabbitMQ data
func (h *Handlers) GetRabbitMQScoreboard(c *gin.Context) {
	// Get consumers from RabbitMQ to derive scoreboard
	consumers, err := h.RabbitMQClient.GetConsumersFromAPI()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to get consumers from RabbitMQ: " + err.Error(),
		})
		return
	}

	// Build scoreboard from consumer data
	scoreboard := make([]map[string]interface{}, 0)

	for _, consumer := range consumers {
		queueData, _ := consumer["queue"].(map[string]interface{})
		queueName, _ := queueData["name"].(string)
		consumerTag, _ := consumer["consumer_tag"].(string)

		// Extract player name from consumer tag (assumes format: playername-worker-*)
		playerName := "unknown"
		if parts := strings.Split(consumerTag, "-"); len(parts) > 0 {
			playerName = parts[0]
		}

		scoreboard = append(scoreboard, map[string]interface{}{
			"player":       playerName,
			"queue":        queueName,
			"consumer_tag": consumerTag,
			"status":       "active",
			"score":        0, // Could be derived from message counts
		})
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"scoreboard":       scoreboard,
			"total_players":    len(scoreboard),
			"source":           "direct_rabbitmq_go_client",
			"educational_note": "Scoreboard derived from RabbitMQ consumer data",
		},
		Message: fmt.Sprintf("Scoreboard with %d active players", len(scoreboard)),
	})
}
