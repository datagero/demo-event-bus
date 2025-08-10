package clients

import (
	"bytes"
	"demo-event-bus-api/internal/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PythonClient handles communication with the Python service
type PythonClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPythonClient creates a new Python service client
func NewPythonClient(baseURL string) *PythonClient {
	return &PythonClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Health checks the health of the Python service
func (c *PythonClient) Health() (*models.HealthResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/health")
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	var health models.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode health response: %w", err)
	}

	return &health, nil
}

// GetGameState retrieves the current game state from Python
func (c *PythonClient) GetGameState() (map[string]interface{}, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/state")
	if err != nil {
		return nil, fmt.Errorf("get game state failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get game state returned status %d", resp.StatusCode)
	}

	var state map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode game state: %w", err)
	}

	return state, nil
}

// RunScenario triggers a scenario in the Python service
func (c *PythonClient) RunScenario(scenarioName string) error {
	payload := map[string]string{"scenario": scenarioName}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal scenario request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/scenario/run",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("run scenario failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("run scenario returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetCardGameStatus retrieves card game status from Python
func (c *PythonClient) GetCardGameStatus() (map[string]interface{}, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/cardgame/status")
	if err != nil {
		return nil, fmt.Errorf("get card game status failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get card game status returned status %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode card game status: %w", err)
	}

	return status, nil
}

// PublishMessage publishes a message via the Python service
func (c *PythonClient) PublishMessage(routingKey string, payload map[string]interface{}) error {
	requestBody := map[string]interface{}{
		"routing_key": routingKey,
		"payload":     payload,
	}

	data, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal publish request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/publish",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("publish message failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("publish message returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
