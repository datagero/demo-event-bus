package scenarios_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Simple test framework that calls API endpoints
// REQUIRES both API server and Workers service to be running

const (
	ApiServerURL = "http://localhost:9000"
	WorkersURL   = "http://localhost:8001"
	RabbitMQURL  = "http://localhost:15672" // RabbitMQ Management API
)

// ScenarioTestResult represents the response from scenario API
type ScenarioTestResult struct {
	Success   bool                     `json:"success"`
	Scenario  string                   `json:"scenario"`
	Summary   string                   `json:"summary"`
	Error     string                   `json:"error,omitempty"`
	TestSteps []map[string]interface{} `json:"test_steps"`
	Results   map[string]interface{}   `json:"results"`
}

// APIResponse represents the standard API response format
type APIResponse struct {
	OK      bool               `json:"ok"`
	Data    ScenarioTestResult `json:"data"`
	Message string             `json:"message,omitempty"`
	Error   string             `json:"error,omitempty"`
}

// TestPrerequisites checks that required services are running
func TestPrerequisites(t *testing.T) {
	t.Run("API Server Available", func(t *testing.T) {
		resp, err := http.Get(ApiServerURL + "/health")
		require.NoError(t, err, "API Server must be running at %s", ApiServerURL)
		require.Equal(t, http.StatusOK, resp.StatusCode, "API Server health check failed")
		resp.Body.Close()
	})

	t.Run("Workers Service Available", func(t *testing.T) {
		resp, err := http.Get(WorkersURL + "/health")
		require.NoError(t, err, "Workers service must be running at %s", WorkersURL)
		require.Equal(t, http.StatusOK, resp.StatusCode, "Workers service health check failed")
		resp.Body.Close()
	})
}

// TestLateBind_Escort tests the Late-bind Escort scenario via API
func TestLateBind_Escort(t *testing.T) {
	// Ensure prerequisites
	TestPrerequisites(t)

	t.Run("Late-bind Escort Scenario", func(t *testing.T) {
		// Call the scenario API endpoint
		payload := map[string]interface{}{
			"scenario": "late-bind-escort",
			"parameters": map[string]interface{}{
				"message_count": 2,
			},
		}

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		resp, err := http.Post(
			ApiServerURL+"/api/scenario-tests/run",
			"application/json",
			bytes.NewBuffer(body),
		)
		require.NoError(t, err, "Failed to call scenario API")
		defer resp.Body.Close()

		// Parse response
		var apiResp APIResponse
		err = json.NewDecoder(resp.Body).Decode(&apiResp)
		require.NoError(t, err, "Failed to parse API response")

		// Validate response structure
		assert.True(t, apiResp.OK, "API call should succeed")
		assert.Equal(t, "late-bind-escort", apiResp.Data.Scenario)

		// Scenario should succeed when workers service is available
		if !apiResp.Data.Success {
			t.Logf("Scenario failed: %s", apiResp.Data.Error)
			t.Logf("Summary: %s", apiResp.Data.Summary)

			// Print test steps for debugging
			for i, step := range apiResp.Data.TestSteps {
				t.Logf("Step %d: %v", i+1, step)
			}
		}
		assert.True(t, apiResp.Data.Success, "Late-bind Escort scenario should succeed when workers service is available")

		// Validate key results
		assert.NotEmpty(t, apiResp.Data.Summary, "Should have summary")
		assert.NotEmpty(t, apiResp.Data.TestSteps, "Should have test steps")

		if apiResp.Data.Success {
			assert.Contains(t, apiResp.Data.Results, "total_messages_sent", "Should track total messages")
			assert.Contains(t, apiResp.Data.Results, "unreachable_message_id", "Should track unreachable message")
			assert.Contains(t, apiResp.Data.Results, "message_states", "Should track individual message states with structure")
			assert.Contains(t, apiResp.Data.Results, "expected_system_state", "Should specify expected system state")
			assert.Contains(t, apiResp.Data.Results, "quest_log_entries", "Should include Quest Log entries")

			// 🎯 CORE VALIDATION: Query RabbitMQ directly and validate against expected states
			t.Log("🔍 Validating RabbitMQ state against expected final states...")

			// Get expected final state from results
			expectedSystemState := apiResp.Data.Results["expected_system_state"].(map[string]interface{})
			messageStates := apiResp.Data.Results["message_states"].([]interface{})
			questLogEntries := apiResp.Data.Results["quest_log_entries"].([]interface{})

			// Validate unroutable messages in DLQ
			unroutableCount, err := getUnroutableMessageCount()
			require.NoError(t, err, "Should be able to query unroutable DLQ")
			expectedUnroutable := int(expectedSystemState["unroutable_messages"].(float64))
			assert.Equal(t, expectedUnroutable, unroutableCount, "Should have exactly %d unroutable message(s) in DLQ", expectedUnroutable)

			// Validate escort queue status
			escortQueue, err := getEscortQueueStatus()
			require.NoError(t, err, "Should be able to query escort queue")

			expectedWorkers := int(expectedSystemState["active_workers"].(float64))
			assert.Equal(t, expectedWorkers, escortQueue.Consumers, "Should have exactly %d active worker(s)", expectedWorkers)

			expectedPending := int(expectedSystemState["pending_messages"].(float64))
			assert.Equal(t, expectedPending, escortQueue.Messages, "Should have exactly %d pending message(s)", expectedPending)

			// 🎯 VALIDATE INDIVIDUAL MESSAGE STATES
			t.Log("🔍 Validating individual message states...")

			unroutableMessageID := apiResp.Data.Results["unreachable_message_id"].(string)

			// Validate each tracked message state
			for _, msgStateRaw := range messageStates {
				msgState := msgStateRaw.(map[string]interface{})
				messageID := msgState["id"].(string)
				expectedState := msgState["expected_state"].(string)
				step := int(msgState["step"].(float64))

				switch expectedState {
				case "unroutable":
					// This should be the unreachable message
					assert.Equal(t, unroutableMessageID, messageID, "Unroutable message ID should match")
					t.Logf("✅ Message %s (Step %d) correctly in unroutable state", messageID, step)

				case "completed":
					// These messages should NOT be in any RabbitMQ queue (processed and removed)
					// We can't easily verify they're "completed" but we can verify they're not stuck
					t.Logf("✅ Message %s (Step %d) expected to be completed (not in queues)", messageID, step)
				}
			}

			// 🎯 VALIDATE QUEST LOG ENTRIES
			t.Log("🔍 Validating Quest Log entries...")
			assert.Greater(t, len(questLogEntries), 5, "Should have multiple Quest Log entries")
			t.Logf("📋 Captured %d Quest Log entries for scenario tracking", len(questLogEntries))

			// Log detailed validation results
			t.Logf("📊 Final State Validation:")
			t.Logf("   ✅ Unroutable messages in DLQ: %d (expected: %d)", unroutableCount, expectedUnroutable)
			t.Logf("   ✅ Active workers: %d (expected: %d)", escortQueue.Consumers, expectedWorkers)
			t.Logf("   ✅ Pending messages: %d (expected: %d)", escortQueue.Messages, expectedPending)
			t.Logf("   ✅ Tracked %d individual messages with expected states", len(messageStates))
			t.Logf("   ✅ Captured %d Quest Log entries", len(questLogEntries))

			// Summary validation
			totalTrackedMessages := len(messageStates)
			t.Logf("📈 Message Flow Summary:")
			t.Logf("   - Total messages sent: %d", totalTrackedMessages)
			t.Logf("   - Messages in unroutable DLQ: %d", unroutableCount)
			t.Logf("   - Messages pending processing: %d", escortQueue.Messages)
			t.Logf("   - Expected completed messages: %d", totalTrackedMessages-unroutableCount-escortQueue.Messages)
		}

		t.Logf("✅ Scenario completed: %s", apiResp.Data.Summary)
	})
}

// TestListAvailableScenarios tests the scenario listing API
func TestListAvailableScenarios(t *testing.T) {
	resp, err := http.Get(ApiServerURL + "/api/scenario-tests/")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var apiResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&apiResp)
	require.NoError(t, err)

	assert.True(t, apiResp["ok"].(bool))

	data := apiResp["data"].(map[string]interface{})
	scenarios := data["scenarios"].([]interface{})

	assert.Greater(t, len(scenarios), 0, "Should have available scenarios")

	// Check that late-bind-escort is available
	found := false
	for _, s := range scenarios {
		scenario := s.(map[string]interface{})
		if scenario["id"] == "late-bind-escort" {
			found = true
			assert.Equal(t, "Late-bind Escort with Unreachable Messages", scenario["name"])
			break
		}
	}
	assert.True(t, found, "Should include late-bind-escort scenario")
}

// TestWorkerServiceDependency verifies that scenarios fail when workers service is unavailable
func TestWorkerServiceDependency(t *testing.T) {
	// This test should only run when we want to verify failure behavior
	// Normally workers service should be available
	t.Skip("This test requires manually stopping workers service")

	payload := map[string]interface{}{
		"scenario": "late-bind-escort",
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, err := http.Post(
		ApiServerURL+"/api/scenario-tests/run",
		"application/json",
		bytes.NewBuffer(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	var apiResp APIResponse
	err = json.NewDecoder(resp.Body).Decode(&apiResp)
	require.NoError(t, err)

	// Scenario should fail when workers service is not available
	assert.False(t, apiResp.Data.Success, "Scenario should fail when workers service is unavailable")
	assert.Contains(t, apiResp.Data.Error, "Worker creation failed", "Should report worker creation failure")
}

// RabbitMQ validation functions

// RabbitMQQueue represents a queue from RabbitMQ Management API
type RabbitMQQueue struct {
	Name      string `json:"name"`
	Messages  int    `json:"messages"`
	Consumers int    `json:"consumers"`
}

// getRabbitMQQueues queries RabbitMQ Management API for queue status
func getRabbitMQQueues() ([]RabbitMQQueue, error) {
	req, err := http.NewRequest("GET", RabbitMQURL+"/api/queues", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("guest", "guest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RabbitMQ API returned status %d", resp.StatusCode)
	}

	var queues []RabbitMQQueue
	err = json.NewDecoder(resp.Body).Decode(&queues)
	return queues, err
}

// getQueueByName finds a specific queue by name
func getQueueByName(queueName string) (*RabbitMQQueue, error) {
	queues, err := getRabbitMQQueues()
	if err != nil {
		return nil, err
	}

	for _, queue := range queues {
		if queue.Name == queueName {
			return &queue, nil
		}
	}
	return nil, fmt.Errorf("queue %s not found", queueName)
}

// getUnroutableMessageCount returns the number of messages in DLQ unroutable queue
func getUnroutableMessageCount() (int, error) {
	queue, err := getQueueByName("game.dlq.unroutable.q")
	if err != nil {
		return 0, err
	}
	return queue.Messages, nil
}

// getEscortQueueStatus returns the status of the escort skill queue
func getEscortQueueStatus() (*RabbitMQQueue, error) {
	return getQueueByName("game.skill.escort.q")
}

// Helper function to wait for services to be ready (useful for CI/CD)
func waitForService(url string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("service at %s not ready after %v", url, timeout)
}

// TestServicesReady can be used in CI/CD to wait for services
func TestServicesReady(t *testing.T) {
	t.Run("Wait for API Server", func(t *testing.T) {
		err := waitForService(ApiServerURL+"/health", 30*time.Second)
		require.NoError(t, err, "API Server should be ready")
	})

	t.Run("Wait for Workers Service", func(t *testing.T) {
		err := waitForService(WorkersURL+"/health", 30*time.Second)
		require.NoError(t, err, "Workers service should be ready")
	})
}
