package handlers

import (
	"demo-event-bus-api/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetChaosStatus retrieves chaos manager status
func (h *Handlers) GetChaosStatus(c *gin.Context) {
	status, err := h.WorkersClient.GetChaosStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    status,
	})
}

// ArmChaos arms the chaos manager
func (h *Handlers) ArmChaos(c *gin.Context) {
	var req map[string]interface{}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Enable chaos
	req["enabled"] = true

	if err := h.WorkersClient.SetChaosConfig(req); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Chaos armed successfully",
	})
}

// DisarmChaos disarms the chaos manager
func (h *Handlers) DisarmChaos(c *gin.Context) {
	config := map[string]interface{}{
		"enabled": false,
	}

	if err := h.WorkersClient.SetChaosConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Chaos disarmed successfully",
	})
}

// SetChaosConfig configures the chaos manager
func (h *Handlers) SetChaosConfig(c *gin.Context) {
	var req map[string]interface{}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if err := h.WorkersClient.SetChaosConfig(req); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Chaos configuration updated successfully",
	})
}
