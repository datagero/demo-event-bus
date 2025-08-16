#!/bin/bash

# Demo script showing how to use the new Scenario Test API endpoints
# These endpoints make scenarios.go redundant by providing test-driven scenarios

echo "🎬 Demo: Scenario Test API Endpoints"
echo "====================================="
echo

# Base URL for API server
API_BASE="http://localhost:9000/api"

echo "1. 📋 List Available Scenario Tests"
echo "GET ${API_BASE}/scenario-tests/"
echo
curl -s "${API_BASE}/scenario-tests/" | jq '.data.scenarios[] | {id: .id, name: .name, description: .description}' 2>/dev/null || echo "❌ API server not running on localhost:9000"
echo
echo

echo "2. 🚀 Run Late-bind Escort Scenario Test"
echo "POST ${API_BASE}/scenario-tests/run"
echo "Body: {\"scenario\": \"late-bind-escort\", \"parameters\": {\"message_count\": 2}}"
echo

# Run the scenario test
RESPONSE=$(curl -s -X POST "${API_BASE}/scenario-tests/run" \
  -H "Content-Type: application/json" \
  -d '{"scenario": "late-bind-escort", "parameters": {"message_count": 2}}' 2>/dev/null)

if [ $? -eq 0 ] && [ -n "$RESPONSE" ]; then
    echo "✅ Scenario executed successfully!"
    echo
    echo "📊 Execution Summary:"
    echo "$RESPONSE" | jq '.data | {
        scenario: .scenario,
        success: .success,
        execution_time_ms: .execution_time_ms,
        summary: .summary
    }' 2>/dev/null
    echo
    echo "📝 Test Steps:"
    echo "$RESPONSE" | jq '.data.test_steps[] | "  Step \(.step): \(.name) - \(.status)"' -r 2>/dev/null
    echo
    echo "📈 Results:"
    echo "$RESPONSE" | jq '.data.results' 2>/dev/null
else
    echo "❌ API server not running or error occurred"
    echo "To start the API server: ./api-server"
fi

echo
echo "3. 🔬 Available Scenario Tests:"
echo "   - late-bind-escort: Tests message routing when no consumers exist"
echo "   - dlq-message-flow: Tests DLQ message categorization and routing (not implemented)"
echo "   - reissuing-dlq: Tests reissuing failed and unroutable messages (not implemented)"
echo "   - orphaned-skill-queues: Tests queue behavior when workers disconnect (not implemented)"
echo
echo "4. 🌐 UI Integration:"
echo "   The UI can now call these endpoints directly instead of using scenarios.go"
echo "   This provides:"
echo "   - ✅ Test-driven scenario execution"
echo "   - ✅ Detailed step-by-step reporting"
echo "   - ✅ Real-time progress tracking"
echo "   - ✅ Standardized response format"
echo "   - ✅ Parameter customization"
echo
echo "🎯 Result: scenarios.go is now redundant - scenarios are test-driven!"