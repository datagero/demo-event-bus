package models

import "time"

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp float64                `json:"ts"`
	// Additional fields from Python implementation
	Scoreboard       map[string]int         `json:"scoreboard,omitempty"`
	Fails            map[string]int         `json:"fails,omitempty"`
	Roster           map[string]interface{} `json:"roster,omitempty"`
	RoutingMode      string                 `json:"routing_mode,omitempty"`
	Metrics          map[string]interface{} `json:"metrics,omitempty"`
	PlayerStats      map[string]interface{} `json:"player_stats,omitempty"`
	GoWorkersEnabled bool                   `json:"go_workers_enabled,omitempty"`
}

// Player represents a player/worker in the system
type Player struct {
	Name            string    `json:"name"`
	Skills          []string  `json:"skills"`
	Status          string    `json:"status"`
	Workers         int       `json:"workers"`
	FailPct         float64   `json:"fail_pct"`
	SpeedMultiplier float64   `json:"speed_multiplier"`
	Type            string    `json:"type"` // "go" or "python"
	LastSeen        time.Time `json:"last_seen"`
}

// Quest represents a quest/message in the system
type Quest struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"`
	AssignedTo string                 `json:"assigned_to,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Points     int                    `json:"points,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Services  map[string]interface{} `json:"services"`
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"ok"` // Frontend expects "ok" field
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}
