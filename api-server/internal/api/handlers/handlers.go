package handlers

import (
	"demo-event-bus-api/internal/clients"
	"demo-event-bus-api/internal/config"
	"demo-event-bus-api/internal/models"
	"demo-event-bus-api/internal/websocket"
	"log"
	"strings"
	"sync"
	"time"
)

// Handlers contains all the API endpoint handlers
type Handlers struct {
	PythonClient   *clients.PythonClient
	WorkersClient  *clients.WorkersClient
	WSHub          *websocket.Hub
	Config         *config.Config
	RabbitMQClient *clients.RabbitMQClient

	// live in-memory stats for UI graphs
	statsMu     sync.RWMutex
	playerStats map[string]map[string]int // name -> {accepted, completed, failed, inflight}
}

// broadcastMessage is a helper to broadcast WebSocket messages
func (h *Handlers) broadcastMessage(msgType string, payload map[string]interface{}) {
	msg := &models.WebSocketMessage{
		Type:    msgType,
		Payload: payload,
	}
	h.WSHub.BroadcastMessage(msg)
}

// StartTicker starts a periodic broadcaster that emits "tick" messages
// with metrics (derived from RabbitMQ), a lightweight roster snapshot,
// and accumulated per-player stats for the throughput/activity graphs.
func (h *Handlers) StartTicker() {
	// initialize stats map if needed
	h.statsMu.Lock()
	if h.playerStats == nil {
		h.playerStats = make(map[string]map[string]int)
	}
	h.statsMu.Unlock()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			// Derive metrics directly from RabbitMQ
			metrics, err := h.RabbitMQClient.DeriveMetricsFromRabbitMQ()
			if err != nil {
				log.Printf("❌ [Ticker] DeriveMetrics error: %v", err)
			}

			// Build roster from RabbitMQ consumers (each unique player online)
			var roster map[string]interface{}
			consumers, err := h.RabbitMQClient.GetConsumersFromAPI()
			if err != nil {
				log.Printf("❌ [Ticker] GetConsumers error: %v", err)
			} else if len(consumers) > 0 {
				roster = make(map[string]interface{})
				for _, c := range consumers {
					tag, _ := c["consumer_tag"].(string)
					if tag == "" {
						continue
					}
					// expected tags like: player-skill-worker-N or player-worker-N
					name := tag
					if parts := strings.Split(tag, "-"); len(parts) > 0 {
						name = parts[0]
					}
					roster[name] = map[string]interface{}{
						"status": "online",
						"type":   "go",
					}
				}
			}

			// Fallback roster from known player stats if management API unavailable
			if roster == nil {
				psTmp := h.getPlayerStatsSnapshot()
				if len(psTmp) > 0 {
					roster = make(map[string]interface{})
					for name := range psTmp {
						roster[name] = map[string]interface{}{"status": "online", "type": "go"}
					}
				}
			}

			// Snapshot player stats
			ps := h.getPlayerStatsSnapshot()

			// Broadcast one compact message with nested payload.ts for UI
			h.WSHub.BroadcastMessage(&models.WebSocketMessage{
				Type:        "tick",
				Payload:     map[string]interface{}{"ts": float64(time.Now().Unix())},
				Metrics:     metrics,
				Roster:      roster, // omitted if nil
				PlayerStats: ps,
			})
		}
	}()
}

// getPlayerStatsSnapshot returns a read-only copy of player stats
func (h *Handlers) getPlayerStatsSnapshot() map[string]interface{} {
	h.statsMu.RLock()
	defer h.statsMu.RUnlock()
	out := make(map[string]interface{}, len(h.playerStats))
	for name, stats := range h.playerStats {
		// copy map so callers can't mutate internal state
		m := map[string]int{
			"accepted":  stats["accepted"],
			"completed": stats["completed"],
			"failed":    stats["failed"],
			"inflight":  stats["inflight"],
		}
		out[name] = m
	}
	return out
}

// updatePlayerStat increments a specific stat counter for a player
func (h *Handlers) updatePlayerStat(player string, field string, delta int) {
	if player == "" {
		return
	}
	h.statsMu.Lock()
	if h.playerStats == nil {
		h.playerStats = make(map[string]map[string]int)
	}
	if h.playerStats[player] == nil {
		h.playerStats[player] = map[string]int{"accepted": 0, "completed": 0, "failed": 0, "inflight": 0}
	}
	h.playerStats[player][field] += delta
	if field == "completed" || field == "failed" {
		// on terminal outcomes, decrement inflight but never below zero
		if h.playerStats[player]["inflight"] > 0 {
			h.playerStats[player]["inflight"] -= 1
		}
	}
	h.statsMu.Unlock()
}

// ensurePlayer initializes a player entry in stats so UI can render immediately
func (h *Handlers) ensurePlayer(player string) {
	if player == "" {
		return
	}
	h.statsMu.Lock()
	if h.playerStats == nil {
		h.playerStats = make(map[string]map[string]int)
	}
	if h.playerStats[player] == nil {
		h.playerStats[player] = map[string]int{"accepted": 0, "completed": 0, "failed": 0, "inflight": 0}
	}
	h.statsMu.Unlock()
}
