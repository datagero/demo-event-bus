package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PublishMessage publishes a single message
func (h *Handlers) PublishMessage(c *gin.Context) {
	var req struct {
		RoutingKey string                 `json:"routing_key" binding:"required"`
		Payload    map[string]interface{} `json:"payload" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.PythonClient.PublishMessage(req.RoutingKey, req.Payload); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Message published successfully",
	})
}

// PublishWave publishes multiple messages
func (h *Handlers) PublishWave(c *gin.Context) {
	// Delegate to Python service for now
	c.JSON(http.StatusNotImplemented, models.APIResponse{
		Success: false,
		Error:   "Publish wave functionality not yet migrated to Go API",
		Message: "Use Python API /api/publish/wave for now",
	})
}

// StartMaster starts the master publisher
func (h *Handlers) StartMaster(c *gin.Context) {
	// Delegate to Python service for now
	c.JSON(http.StatusNotImplemented, models.APIResponse{
		Success: false,
		Error:   "Master publisher functionality not yet migrated to Go API",
		Message: "Use Python API /api/master/start for now",
	})
}

// Message list handlers (all delegate to Python for now)
func (h *Handlers) ListPendingMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "pending messages list")
}

func (h *Handlers) ReissuePendingMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue pending messages")
}

func (h *Handlers) ReissueAllPendingMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue all pending messages")
}

func (h *Handlers) ListFailedMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "failed messages list")
}

func (h *Handlers) ReissueFailedMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue failed messages")
}

func (h *Handlers) ReissueAllFailedMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue all failed messages")
}

func (h *Handlers) ListDLQMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "DLQ messages list")
}

func (h *Handlers) ReissueDLQMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue DLQ messages")
}

func (h *Handlers) ReissueAllDLQMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue all DLQ messages")
}

func (h *Handlers) ListUnroutableMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "unroutable messages list")
}

func (h *Handlers) ReissueUnroutableMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue unroutable messages")
}

func (h *Handlers) ReissueAllUnroutableMessages(c *gin.Context) {
	h.delegateToTypeNotImplemented(c, "reissue all unroutable messages")
}

// Helper function for not-yet-implemented endpoints
func (h *Handlers) delegateToTypeNotImplemented(c *gin.Context, functionality string) {
	c.JSON(http.StatusNotImplemented, models.APIResponse{
		Success: false,
		Error:   functionality + " functionality not yet migrated to Go API",
		Message: "Use corresponding Python API endpoint for now",
	})
}
