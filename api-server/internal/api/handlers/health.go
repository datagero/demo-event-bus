package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Health handles health check requests
func (h *Handlers) Health(c *gin.Context) {
	services := make(map[string]string)

	// Check Python service
	if _, err := h.PythonClient.Health(); err != nil {
		services["python"] = "down"
	} else {
		services["python"] = "up"
	}

	// Check Workers service
	if _, err := h.WorkersClient.GetStatus(); err != nil {
		services["workers"] = "down"
	} else {
		services["workers"] = "up"
	}

	services["api"] = "up"
	services["websocket"] = "up"

	response := models.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Services:  services,
	}

	c.JSON(http.StatusOK, response)
}
