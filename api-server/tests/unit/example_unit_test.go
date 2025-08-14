package handlers_test

import (
	"testing"

	"demo-event-bus-api/tests/testdata"

	"github.com/stretchr/testify/assert"
)

// TestExampleUnit demonstrates how to write a simple unit test
func TestExampleUnit(t *testing.T) {
	// This is a unit test - no external services required
	// It should be fast and test isolated logic

	// Use fixtures for consistent test data
	player := testdata.TestPlayers[0] // Alice, Warrior

	// Test some validation logic
	assert.Equal(t, "Alice", player.Name)
	assert.Equal(t, "Warrior", player.Class)

	// Test request validation
	err := testdata.ValidatePlayerRequest(testdata.ValidPlayerStartRequest)
	assert.NoError(t, err)

	err = testdata.ValidatePlayerRequest(testdata.InvalidPlayerStartRequest)
	assert.Error(t, err)
}

// TestTestFramework demonstrates the test framework capabilities
func TestTestFramework(t *testing.T) {
	// Create test context (but don't use external services)
	tc := testdata.NewTestContext(t)

	// Test the service health check method
	available, missing := tc.ServiceHealthCheck()

	// In unit tests, we don't require services to be available
	// Just test that the health check works
	assert.NotNil(t, available)
	assert.IsType(t, []string{}, missing)

	// Test fixtures
	assert.Len(t, testdata.TestPlayers, 4)
	assert.Len(t, testdata.TestWorkers, 3)
	assert.Equal(t, "/reset", testdata.APIEndpoints.Reset)
}

// TestValidationHelpers shows how to test validation logic
func TestValidationHelpers(t *testing.T) {
	tests := []struct {
		name        string
		request     map[string]interface{}
		expectError bool
		description string
	}{
		{
			name:        "Valid player start request",
			request:     testdata.ValidPlayerStartRequest,
			expectError: false,
			description: "Should accept valid player start request",
		},
		{
			name:        "Invalid player start request",
			request:     testdata.InvalidPlayerStartRequest,
			expectError: true,
			description: "Should reject request with wrong field names",
		},
		{
			name: "Missing player field",
			request: map[string]interface{}{
				"skills": []string{"gather"},
			},
			expectError: true,
			description: "Should reject request missing player field",
		},
		{
			name: "Missing skills field",
			request: map[string]interface{}{
				"player": "alice",
			},
			expectError: true,
			description: "Should reject request missing skills field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testdata.ValidatePlayerRequest(tt.request)
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}
