package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"demo-event-bus-workers/consumer"
)

// Action represents different chaos actions
type Action string

const (
	ActionDrop       Action = "drop"       // Real disconnect + auto-reconnect
	ActionPause      Action = "pause"      // Pause processing
	ActionDisconnect Action = "disconnect" // Disconnect without reconnect
	ActionSlow       Action = "slow"       // Slow down processing
	ActionFail       Action = "fail"       // Increase failure rate
)

// Manager manages chaos actions on workers
type Manager struct {
	workers map[string]*consumer.Worker
	mu      sync.RWMutex
	config  Config
}

// Config holds chaos configuration
type Config struct {
	Enabled           bool          `json:"enabled"`
	Action            Action        `json:"action"`
	TargetPlayer      string        `json:"target_player"`     // "" for all players
	AutoReconnectTime time.Duration `json:"auto_reconnect_ms"` // For drop action
	SlowMultiplier    float64       `json:"slow_multiplier"`   // For slow action
	FailIncrease      float64       `json:"fail_increase"`     // For fail action
}

// NewManager creates a new chaos manager
func NewManager() *Manager {
	return &Manager{
		workers: make(map[string]*consumer.Worker),
		config: Config{
			AutoReconnectTime: 3 * time.Second,
			SlowMultiplier:    3.0,
			FailIncrease:      0.5,
		},
	}
}

// RegisterWorker registers a worker for chaos actions
func (m *Manager) RegisterWorker(playerName string, worker *consumer.Worker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workers[playerName] = worker
	log.Printf("🎭 [Chaos] Registered worker: %s", playerName)
}

// UnregisterWorker removes a worker from chaos management
func (m *Manager) UnregisterWorker(playerName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workers, playerName)
	log.Printf("🎭 [Chaos] Unregistered worker: %s", playerName)
}

// SetConfig updates the chaos configuration
func (m *Manager) SetConfig(config Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	log.Printf("🎭 [Chaos] Config updated: %+v", config)
}

// TriggerAction triggers a chaos action
func (m *Manager) TriggerAction(action Action, targetPlayer string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.Enabled {
		return fmt.Errorf("chaos is disabled")
	}

	var targets []*consumer.Worker
	if targetPlayer != "" {
		// Target specific player
		if worker, exists := m.workers[targetPlayer]; exists {
			targets = append(targets, worker)
		} else {
			return fmt.Errorf("player %s not found", targetPlayer)
		}
	} else {
		// Target all players
		for _, worker := range m.workers {
			targets = append(targets, worker)
		}
	}

	for _, worker := range targets {
		go m.executeAction(action, worker)
	}

	return nil
}

// AutoTrigger automatically triggers chaos actions
func (m *Manager) AutoTrigger(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			if m.config.Enabled && len(m.workers) > 0 {
				// Random chance to trigger chaos
				if rand.Float64() < 0.1 { // 10% chance every 10 seconds
					players := make([]string, 0, len(m.workers))
					for player := range m.workers {
						players = append(players, player)
					}

					if len(players) > 0 {
						targetPlayer := players[rand.Intn(len(players))]
						action := m.config.Action
						if action == "" {
							// Random action
							actions := []Action{ActionDrop, ActionPause, ActionSlow}
							action = actions[rand.Intn(len(actions))]
						}

						log.Printf("🎭 [Chaos] Auto-triggering %s on %s", action, targetPlayer)
						go m.executeAction(action, m.workers[targetPlayer])
					}
				}
			}
			m.mu.RUnlock()
		}
	}
}

// executeAction executes a specific chaos action on a worker
func (m *Manager) executeAction(action Action, worker *consumer.Worker) {
	switch action {
	case ActionDrop:
		m.actionDrop(worker)
	case ActionPause:
		m.actionPause(worker)
	case ActionDisconnect:
		m.actionDisconnect(worker)
	case ActionSlow:
		m.actionSlow(worker)
	case ActionFail:
		m.actionFail(worker)
	default:
		log.Printf("🎭 [Chaos] Unknown action: %s", action)
	}
}

// actionDrop implements real disconnect + auto-reconnect
func (m *Manager) actionDrop(worker *consumer.Worker) {
	// Find player name from the workers map
	var playerName string
	for name, w := range m.workers {
		if w == worker {
			playerName = name
			break
		}
	}

	log.Printf("⚠️  [Chaos] DROP: Disconnecting %s for %v", playerName, m.config.AutoReconnectTime)

	// Notify Python server about disconnect
	m.notifyChaosEvent(playerName, "disconnect", fmt.Sprintf("Chaos DROP: disconnected for %v", m.config.AutoReconnectTime))

	// Stop the worker (real disconnect)
	worker.Stop()

	// Wait for auto-reconnect time
	time.Sleep(m.config.AutoReconnectTime)

	// Restart the worker (reconnect)
	log.Printf("🔄 [Chaos] DROP: Reconnecting %s", playerName)
	err := worker.Start()
	if err != nil {
		log.Printf("❌ [Chaos] DROP: Failed to reconnect %s: %v", playerName, err)
		m.notifyChaosEvent(playerName, "reconnect_failed", fmt.Sprintf("Chaos DROP: reconnect failed - %v", err))
	} else {
		log.Printf("✅ [Chaos] DROP: Successfully reconnected %s", playerName)
		m.notifyChaosEvent(playerName, "reconnect", "Chaos DROP: reconnected successfully")
	}
}

// actionPause pauses the worker temporarily
func (m *Manager) actionPause(worker *consumer.Worker) {
	pauseDuration := 5 * time.Second
	log.Printf("🎭 [Chaos] PAUSE: Pausing worker for %v", pauseDuration)

	worker.Pause()
	time.Sleep(pauseDuration)
	worker.Resume()

	log.Printf("🎭 [Chaos] PAUSE: Resumed worker")
}

// actionDisconnect disconnects the worker permanently
func (m *Manager) actionDisconnect(worker *consumer.Worker) {
	log.Printf("🎭 [Chaos] DISCONNECT: Permanently disconnecting worker")
	worker.Stop()
}

// actionSlow slows down the worker
func (m *Manager) actionSlow(worker *consumer.Worker) {
	log.Printf("🎭 [Chaos] SLOW: Not implemented in this version")
	// This would require modifying the worker's speed multiplier
	// Could be implemented by updating the worker config
}

// actionFail increases failure rate
func (m *Manager) actionFail(worker *consumer.Worker) {
	log.Printf("🎭 [Chaos] FAIL: Not implemented in this version")
	// This would require modifying the worker's failure rate
}

// notifyChaosEvent sends chaos events to the Python server via webhook
func (m *Manager) notifyChaosEvent(playerName string, eventType string, description string) {
	// Use a default webhook URL - we'll need to get this from the main server
	// For now, use the standard webhook URL
	webhookURL := "http://localhost:8000/api/go-workers/webhook/events"

	payload := map[string]interface{}{
		"type":        "chaos_event",
		"event":       eventType,
		"player":      playerName,
		"description": description,
		"source":      "go-chaos",
		"timestamp":   time.Now().Unix(),
	}

	// Send webhook notification
	m.sendWebhook(webhookURL, payload)
}

// sendWebhook sends a webhook notification to the Python server
func (m *Manager) sendWebhook(webhookURL string, payload map[string]interface{}) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("❌ [Chaos] Failed to marshal webhook: %v", err)
		return
	}

	// Send async to avoid blocking
	go func() {
		resp, err := http.Post(webhookURL, "application/json", strings.NewReader(string(jsonData)))
		if err != nil {
			log.Printf("❌ [Chaos] Webhook failed: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("⚠️  [Chaos] Webhook returned status: %d", resp.StatusCode)
		}
	}()
}

// GetStatus returns the current chaos status
func (m *Manager) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"enabled":            m.config.Enabled,
		"action":             m.config.Action,
		"target_player":      m.config.TargetPlayer,
		"auto_reconnect_ms":  m.config.AutoReconnectTime.Milliseconds(),
		"registered_workers": len(m.workers),
		"worker_names":       m.getWorkerNames(),
	}
}

// getWorkerNames returns a list of registered worker names
func (m *Manager) getWorkerNames() []string {
	names := make([]string, 0, len(m.workers))
	for name := range m.workers {
		names = append(names, name)
	}
	return names
}
