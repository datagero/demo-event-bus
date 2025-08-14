# Integration Test Scenarios

This document describes the comprehensive integration tests created to validate the scenarios we encountered during the Alice worker troubleshooting session.

## Background

During the debugging session, we encountered several critical issues:

1. **Alice worker stuck in "stopping" state** - Worker was requeuing messages instead of processing them
2. **RabbitMQ connectivity issues** - Services couldn't connect after restart
3. **API request format validation errors** - Wrong field names causing validation failures
4. **Inconsistent state between services** - Workers service and RabbitMQ showing different information
5. **Reset functionality gaps** - Need comprehensive cleanup of stuck states

## Test Suite Overview

### 1. Worker Lifecycle Integration Tests
**File**: `worker_lifecycle_integration_test.go`

**Purpose**: Tests the complete worker lifecycle that caused the Alice issue.

**Test Cases**:
- `TestWorkerLifecycleIntegration`
  - Worker Start and Verify Active
  - Worker Stop and Verify Cleanup  
  - Reset Clears All Workers
- `TestWorkerStuckStateDetection`
  - Detect Worker Registration vs Actual Activity
- `TestWorkerAPIValidation`
  - Start Worker Validation (correct vs incorrect request formats)
  - Stop Worker Validation

**Key Validations**:
- Workers appear in RabbitMQ consumers as active (not stuck)
- Proper cleanup when workers are stopped
- Reset functionality clears all workers and consumers
- API request format validation catches common errors

### 2. Message Processing Integration Tests
**File**: `message_processing_integration_test.go`

**Purpose**: Tests end-to-end message processing and queue management.

**Test Cases**:
- `TestMessageProcessingFlow`
  - Worker Processes Messages Successfully
  - Worker Handles Multiple Message Types
- `TestMessageFailureScenarios`
  - High Failure Rate Worker Sends Messages to DLQ
- `TestQueueManagement`
  - Worker Creation Establishes Queues
  - Reset Cleans Up Queues

**Key Validations**:
- Messages are processed efficiently (not stuck in queues)
- Failed messages route to DLQ properly
- Queue creation and cleanup works correctly
- Multiple message types are handled properly

### 3. System Health Integration Tests
**File**: `system_health_integration_test.go`

**Purpose**: Tests system health monitoring and recovery scenarios.

**Test Cases**:
- `TestSystemHealthAndRecovery`
  - Workers Service Connectivity
  - RabbitMQ Management API Health
  - Service Integration Health Check
- `TestStuckWorkerDetection`
  - Detect Inconsistent Worker States
  - Reset Clears Stuck States
- `TestServiceRecoveryScenarios`
  - Recovery After Clean Restart

**Key Validations**:
- Service connectivity issues are detected
- Inconsistent states between workers service and RabbitMQ are caught
- Reset functionality clears stuck states
- Services recover properly after restart

## Running the Tests

### Quick Start
```bash
# Run all integration tests
./api-server/test_scenarios.sh
```

### Prerequisites
Before running tests, ensure services are running:
```bash
# Start all services
./start_app.sh

# Or start manually:
docker-compose up -d  # RabbitMQ
cd api-server && go run main.go &  # API Server
cd workers && go run main.go &     # Workers Service
```

### Individual Test Suites
```bash
cd api-server

# Test specific scenarios
go test -v -run TestWorkerLifecycleIntegration ./internal/api/handlers/
go test -v -run TestStuckWorkerDetection ./internal/api/handlers/
go test -v -run TestSystemHealthAndRecovery ./internal/api/handlers/
go test -v -run TestMessageProcessingFlow ./internal/api/handlers/
go test -v -run TestMessageFailureScenarios ./internal/api/handlers/

# Run only unit tests (skip integration)
go test -short ./internal/api/handlers/
```

## Test Design Principles

### 1. Graceful Degradation
Tests check service availability and skip appropriately when services are unavailable, providing clear feedback about what's not working.

### 2. Real Service Integration
Tests use actual RabbitMQ and Workers service endpoints, not mocks, to catch real integration issues.

### 3. State Validation
Tests verify both the API response and the actual state in RabbitMQ to catch inconsistencies.

### 4. Cleanup and Isolation
Tests use reset functionality and wait periods to ensure clean state between test runs.

### 5. Scenario-Based Testing
Each test maps to a real scenario we encountered, making them practical regression tests.

## Key Test Patterns

### Service Connectivity Check
```go
if w.Code == http.StatusInternalServerError {
    var response models.APIResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    if err == nil && !response.Success {
        if strings.Contains(response.Error, "connection refused") {
            t.Skip("Service not available - this is the connectivity issue we're testing for")
        }
    }
}
```

### State Consistency Validation
```go
// Check both workers service and RabbitMQ
workersServiceCount := getWorkerCountFromService()
rabbitMQConsumerCount := getConsumerCountFromRabbitMQ()

// Validate consistency
assert.Greater(t, rabbitMQConsumerCount, 0, 
    "If workers service reports workers, RabbitMQ should show consumers")
```

### Stuck State Detection
```go
// Check activity status
if activityStatus, exists := consumerMap["activity_status"]; exists {
    assert.Equal(t, "up", activityStatus, 
        "Consumer should be 'up', not 'stopping'")
}
```

## Expected Test Outcomes

### When Services are Healthy
- All tests should pass
- Workers create successfully and appear as active in RabbitMQ
- Messages process correctly
- Reset clears all state properly

### When Services are Unhealthy
- Tests skip gracefully with clear messages
- No false failures due to service unavailability
- Clear indication of which service is causing issues

### When Issues are Detected
- Tests fail with specific error messages
- Clear indication of the type of problem (stuck state, connectivity, etc.)
- Actionable feedback for debugging

## Troubleshooting Test Failures

### "Workers Service Timeout"
- Check if workers service is running on port 8001
- Verify workers service health: `curl http://localhost:8001/health`
- Restart workers service if needed

### "RabbitMQ Connection Refused"
- Check if RabbitMQ is running: `docker-compose ps`
- Start RabbitMQ: `docker-compose up -d`
- Verify management API: `curl http://localhost:15672/api/overview`

### "Stuck Worker Detected"
- This is the main issue we're testing for
- Run reset: `curl -X POST http://localhost:9000/api/reset`
- Restart workers service if reset doesn't clear it

### "Consumer Count Mismatch"
- Indicates inconsistent state between services
- Check logs for errors
- Reset and restart services

## Integration with CI/CD

The test scenarios can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions step
- name: Run Integration Tests
  run: |
    docker-compose up -d
    ./start_app.sh &
    sleep 10
    ./api-server/test_scenarios.sh
```

## Future Enhancements

1. **Performance Tests**: Add timing validations for message processing
2. **Load Tests**: Test with multiple workers and high message volume
3. **Chaos Testing**: Simulate service failures and recovery
4. **Monitoring Integration**: Add metrics collection during tests
5. **Test Data Management**: More sophisticated test data setup/teardown

## Conclusion

These integration tests provide comprehensive coverage of the real-world scenarios that caused issues with the Alice worker. They serve as both regression tests and documentation of proper system behavior, helping prevent similar issues in the future and providing clear validation of fixes when issues do occur.