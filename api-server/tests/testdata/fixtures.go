package testdata

// Test fixtures and common test data for API server tests

// PlayerData represents test player data
type PlayerData struct {
	Name  string `json:"name"`
	Class string `json:"class"`
}

// WorkerData represents test worker configuration
type WorkerData struct {
	Name        string `json:"name"`
	FailureRate int    `json:"failure_rate"`
}

// MessageData represents test message data
type MessageData struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// QuestData represents test quest wave data
type QuestData struct {
	Type       string `json:"type"`
	Difficulty int    `json:"difficulty"`
	Count      int    `json:"count"`
}

// Test data sets
var (
	// TestPlayers provides consistent test player data
	TestPlayers = []PlayerData{
		{Name: "Alice", Class: "Warrior"},
		{Name: "Bob", Class: "Mage"},
		{Name: "Charlie", Class: "Archer"},
		{Name: "Diana", Class: "Healer"},
	}

	// TestWorkers provides test worker configurations
	TestWorkers = []WorkerData{
		{Name: "worker1", FailureRate: 10},
		{Name: "worker2", FailureRate: 20},
		{Name: "worker3", FailureRate: 0},
	}

	// TestQuests provides test quest wave data
	TestQuests = []QuestData{
		{Type: "gather", Difficulty: 1, Count: 5},
		{Type: "slay", Difficulty: 2, Count: 10},
		{Type: "explore", Difficulty: 3, Count: 15},
	}

	// APIEndpoints provides consistent API endpoint paths
	APIEndpoints = struct {
		Reset             string
		PlayerStart       string
		PlayerStop        string
		PlayersQuickstart string
		MasterOne         string
		MasterStart       string
		DLQList           string
		Health            string
	}{
		Reset:             "/reset",
		PlayerStart:       "/api/player/start",
		PlayerStop:        "/api/player/stop",
		PlayersQuickstart: "/api/players/quickstart",
		MasterOne:         "/api/master/one",
		MasterStart:       "/api/master/start",
		DLQList:           "/api/dlq/list",
		Health:            "/health",
	}
)

// Common test messages
var (
	ValidPlayerStartRequest = map[string]interface{}{
		"player": "alice",
		"skills": []string{"gather", "slay"},
	}

	InvalidPlayerStartRequest = map[string]interface{}{
		"worker_name": "alice", // Wrong field name
		"skills":      []string{"gather"},
	}

	ValidPlayerStopRequest = map[string]interface{}{
		"player": "alice",
	}

	ValidQuickstartRequest = map[string]interface{}{
		"player_count": 3,
		"skills":       []string{"gather", "slay"},
	}
)