package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WorkersClient handles communication with the Go workers service
type WorkersClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewWorkersClient creates a new workers service client
func NewWorkersClient(baseURL string) *WorkersClient {
	return &WorkersClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetStatus retrieves the status of all workers
func (c *WorkersClient) GetStatus() (map[string]interface{}, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/status")
	if err != nil {
		return nil, fmt.Errorf("get workers status failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get workers status returned status %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode workers status: %w", err)
	}

	return status, nil
}

// CreateWorker creates a new worker (replaces StartWorker for cleaner interface)
func (c *WorkersClient) CreateWorker(config map[string]interface{}) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal create worker request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/start",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("create worker failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// StartWorker starts a new worker
func (c *WorkersClient) StartWorker(name string, skills []string, config map[string]interface{}) error {
	payload := map[string]interface{}{
		"name":   name,
		"skills": skills,
		"config": config,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal start worker request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/start",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("start worker failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// StopWorker stops a worker
func (c *WorkersClient) StopWorker(name string) error {
	payload := map[string]string{"name": name}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal stop worker request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/stop",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("stop worker failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stop worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ControlWorker controls a worker (pause/resume)
func (c *WorkersClient) ControlWorker(name string, action string) error {
	payload := map[string]string{
		"name":   name,
		"action": action,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal control worker request: %w", err)
	}

	var endpoint string
	switch action {
	case "pause":
		endpoint = "/pause"
	case "resume":
		endpoint = "/resume"
	default:
		return fmt.Errorf("invalid action: %s", action)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+endpoint,
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("control worker failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("control worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetChaosStatus retrieves chaos manager status
func (c *WorkersClient) GetChaosStatus() (map[string]interface{}, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/status")
	if err != nil {
		return nil, fmt.Errorf("get chaos status failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get chaos status returned status %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode chaos status: %w", err)
	}

	return status, nil
}

// SetChaosConfig configures the chaos manager
func (c *WorkersClient) SetChaosConfig(config map[string]interface{}) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal chaos config: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/chaos/config",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("set chaos config failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set chaos config returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
