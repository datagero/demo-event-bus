# API Server Testing Framework

This directory contains the organized testing framework for the API server, providing clear separation between test code and application logic.

## Directory Structure

```
api-server/tests/
├── unit/           # Unit tests - fast, isolated tests
├── integration/    # Integration tests - test with external services
├── scenarios/      # Scenario tests - end-to-end workflows
├── testdata/       # Test fixtures, helpers, and utilities
└── README.md       # This file
```

## Test Categories

### Unit Tests (`unit/`)
- **Purpose**: Fast, isolated tests that don't require external services
- **Focus**: Individual functions, validation logic, request/response handling
- **Dependencies**: Mock objects, test fixtures
- **Runtime**: Typically < 1 second per test

Example unit tests:
- `validation_unit_test.go` - API request validation logic
- `dlq_unit_test.go` - Dead Letter Queue utility functions

### Integration Tests (`integration/`)
- **Purpose**: Tests that verify interaction with external services
- **Focus**: Database operations, RabbitMQ integration, WebSocket communication
- **Dependencies**: Requires running RabbitMQ, actual network connections
- **Runtime**: Typically 1-10 seconds per test

Example integration tests:
- `dlq_auto_setup_integration_test.go` - DLQ topology setup and management
- `message_processing_integration_test.go` - End-to-end message processing
- `worker_lifecycle_integration_test.go` - Worker start/stop workflows

### Scenario Tests (`scenarios/`)
- **Purpose**: End-to-end business workflow testing
- **Focus**: Complete user scenarios, complex multi-step processes
- **Dependencies**: Full system stack, all services running
- **Runtime**: Typically 10+ seconds per test

Example scenario tests:
- `scenarios_integration_test.go` - Complete game scenarios (player creation, quest waves, etc.)

### Test Data (`testdata/`)
- **Purpose**: Shared utilities, fixtures, and helper functions
- **Contents**:
  - `fixtures.go` - Test data sets (players, workers, quests)
  - `helpers.go` - Testing utilities (TestContext, HTTP helpers, assertions)

## Running Tests

### Using the Test Runner Script
```bash
# Run all tests
./run_tests.sh

# Run specific test type
./run_tests.sh unit
./run_tests.sh integration
./run_tests.sh scenarios

# Run with options
./run_tests.sh -v unit          # Verbose output
./run_tests.sh -c all           # With coverage
./run_tests.sh -t 120s integration  # Custom timeout
```

### Using Go Test Directly
```bash
# Run unit tests only
go test ./tests/unit/... -v

# Run integration tests (requires services)
go test ./tests/integration/... -v

# Run all tests
go test ./tests/... -v

# Run with short mode (skips integration tests)
go test ./tests/... -short

# Run specific test file
go test ./tests/unit/validation_unit_test.go -v
```

## Writing Tests

### Test Structure
All tests use the `handlers_test` package to access both public and internal APIs:

```go
package handlers_test

import (
    "testing"
    "demo-event-bus-api/internal/api/handlers"
    "demo-event-bus-api/tests/testdata"
    "github.com/stretchr/testify/assert"
)

func TestExample(t *testing.T) {
    // Use test helpers
    tc := testdata.NewTestContext(t)
    tc.SetupRoutes()
    
    // Use test fixtures
    player := testdata.TestPlayers[0]
    
    // Make requests
    resp := tc.MakeJSONRequest("POST", "/api/player/start", player)
    
    // Assert results
    tc.AssertStatusCode(resp, 200)
}
```

### Unit Test Guidelines
- Use mocks for external dependencies
- Focus on single function/method testing
- Use `testdata.ValidatePlayerRequest()` for validation testing
- Keep tests fast (< 1 second)

### Integration Test Guidelines
- Use `testdata.IsIntegrationTest(t)` to skip in short mode
- Use `tc.SkipIfNoServices("rabbitmq")` to skip if services unavailable
- Test actual service interactions
- Clean up resources after testing

### Test Fixtures
Use consistent test data from `testdata/fixtures.go`:

```go
// Use predefined test data
player := testdata.TestPlayers[0]           // Alice, Warrior
worker := testdata.TestWorkers[0]           // worker1, 10% failure rate
request := testdata.ValidPlayerStartRequest // Valid JSON request
```

## Dependencies

Tests can import and use:
- Internal packages: `demo-event-bus-api/internal/*`
- Application handlers: `demo-event-bus-api/internal/api/handlers`
- Test utilities: `demo-event-bus-api/tests/testdata`
- Standard testing libraries: `github.com/stretchr/testify`

## Best Practices

1. **Separation of Concerns**: Keep tests separate from application code
2. **Consistent Naming**: Use descriptive test function names
3. **Test Isolation**: Each test should be independent
4. **Resource Cleanup**: Clean up after integration tests
5. **Mock External Services**: Use mocks in unit tests
6. **Use Test Helpers**: Leverage `TestContext` for common operations
7. **Meaningful Assertions**: Use descriptive assertion messages

## Troubleshooting

### Common Issues

1. **Import Errors**: Ensure all imports use the correct module path `demo-event-bus-api`
2. **Service Dependencies**: Integration tests require RabbitMQ to be running
3. **Port Conflicts**: Make sure test ports don't conflict with running services
4. **Test Isolation**: If tests interfere with each other, add proper cleanup

### Debug Mode
```bash
# Run with verbose output and no timeout
go test ./tests/unit/... -v -timeout 0

# Run single test with detailed output
go test ./tests/unit/validation_unit_test.go -v -run TestSpecificFunction
```

This testing framework provides a robust foundation for maintaining the API server quality while keeping test code organized and maintainable.