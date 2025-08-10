package handlers

import (
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/models"
	"demo-event-bus-api/internal/websocket"
)

// Handlers contains all the API endpoint handlers
type Handlers struct {
	PythonClient  *clients.PythonClient
	WorkersClient *clients.WorkersClient
	WSHub         *websocket.Hub
	Config        *config.Config
}

// broadcastMessage is a helper to broadcast WebSocket messages
func (h *Handlers) broadcastMessage(msgType string, payload map[string]interface{}) {
	msg := &models.WebSocketMessage{
		Type:    msgType,
		Payload: payload,
	}
	h.WSHub.BroadcastMessage(msg)
}
