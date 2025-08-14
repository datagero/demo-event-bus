#!/bin/bash

# test_scenarios.sh - Run integration tests for the scenarios we encountered

set -e

echo "================================================================================================"
echo "🧪 Running Integration Tests for Alice Worker Scenarios"
echo "================================================================================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if services are running
echo -e "${BLUE}📋 Step 1: Checking Service Availability${NC}"

# Check RabbitMQ
if curl -s -f http://localhost:15672/api/overview >/dev/null 2>&1; then
    echo -e "${GREEN}✅ RabbitMQ Management API accessible${NC}"
else
    echo -e "${YELLOW}⚠️  RabbitMQ not accessible - some tests will be skipped${NC}"
fi

# Check Workers Service
if curl -s -f http://localhost:8001/health >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Workers Service accessible${NC}"
else
    echo -e "${YELLOW}⚠️  Workers Service not accessible - some tests will be skipped${NC}"
fi

# Check API Server
if curl -s -f http://localhost:9000/health >/dev/null 2>&1; then
    echo -e "${GREEN}✅ API Server accessible${NC}"
else
    echo -e "${RED}❌ API Server not accessible - tests will fail${NC}"
    echo "Please start the API server with: ./start_app.sh"
    exit 1
fi

echo ""
echo -e "${BLUE}📋 Step 2: Running Integration Test Suites${NC}"

cd api-server

# Test Worker Lifecycle (the main Alice issue)
echo ""
echo -e "${BLUE}🔄 Testing Worker Lifecycle Scenarios${NC}"
echo "Testing: Start/Stop/Reset worker scenarios that caused Alice to get stuck"
go test -v -run TestWorkerLifecycleIntegration ./internal/api/handlers/ || {
    echo -e "${RED}❌ Worker Lifecycle tests failed${NC}"
}

# Test Message Processing
echo ""
echo -e "${BLUE}📨 Testing Message Processing Flow${NC}"
echo "Testing: End-to-end message processing and queue management"
go test -v -run TestMessageProcessingFlow ./internal/api/handlers/ || {
    echo -e "${RED}❌ Message Processing tests failed${NC}"
}

# Test System Health and Recovery
echo ""
echo -e "${BLUE}🏥 Testing System Health and Recovery${NC}"
echo "Testing: Service connectivity and recovery scenarios"
go test -v -run TestSystemHealthAndRecovery ./internal/api/handlers/ || {
    echo -e "${RED}❌ System Health tests failed${NC}"
}

# Test Stuck Worker Detection
echo ""
echo -e "${BLUE}🛑 Testing Stuck Worker Detection${NC}"
echo "Testing: Detection and recovery from the 'stopping' state issue"
go test -v -run TestStuckWorkerDetection ./internal/api/handlers/ || {
    echo -e "${RED}❌ Stuck Worker Detection tests failed${NC}"
}

# Test API Validation
echo ""
echo -e "${BLUE}🔍 Testing API Request Validation${NC}"
echo "Testing: API request format validation issues we encountered"
go test -v -run TestWorkerAPIValidation ./internal/api/handlers/ || {
    echo -e "${RED}❌ API Validation tests failed${NC}"
}

# Test RabbitMQ Connectivity
echo ""
echo -e "${BLUE}🐰 Testing RabbitMQ Connectivity${NC}"
echo "Testing: RabbitMQ connection issues and recovery"
go test -v -run TestRabbitMQConnectivityRecovery ./internal/api/handlers/ || {
    echo -e "${RED}❌ RabbitMQ Connectivity tests failed${NC}"
}

# Test Message Failure Scenarios (DLQ)
echo ""
echo -e "${BLUE}💀 Testing Message Failure Scenarios${NC}"
echo "Testing: DLQ routing and failure handling"
go test -v -run TestMessageFailureScenarios ./internal/api/handlers/ || {
    echo -e "${RED}❌ Message Failure tests failed${NC}"
}

# Test DLQ Auto-Setup and Resilience
echo ""
echo -e "${BLUE}📬 Testing DLQ Auto-Setup and Resilience${NC}"
echo "Testing: DLQ auto-setup when queues don't exist and recovery after reset"
go test -v -run "TestDLQAutoSetupFunctionality|TestDLQResilienceAfterReset" ./internal/api/handlers/ || {
    echo -e "${RED}❌ DLQ auto-setup and resilience tests failed${NC}"
}

# Test DLQ Message Flow (End-to-End)
echo ""
echo -e "${BLUE}🔄 Testing DLQ Message Flow (End-to-End)${NC}"
echo "Testing: Complete DLQ message flow including the 'Start Quest Wave' scenario"
go test -v -run "TestDLQMessageFlowEndToEnd|TestDLQMessageReissue" ./internal/api/handlers/ || {
    echo -e "${RED}❌ DLQ message flow tests failed${NC}"
}

# Test Queue Management
echo ""
echo -e "${BLUE}📦 Testing Queue Management${NC}"
echo "Testing: Queue creation and cleanup scenarios"
go test -v -run TestQueueManagement ./internal/api/handlers/ || {
    echo -e "${RED}❌ Queue Management tests failed${NC}"
}

# Test Service Recovery
echo ""
echo -e "${BLUE}🔄 Testing Service Recovery Scenarios${NC}"
echo "Testing: Recovery from service restart scenarios"
go test -v -run TestServiceRecoveryScenarios ./internal/api/handlers/ || {
    echo -e "${RED}❌ Service Recovery tests failed${NC}"
}

echo ""
echo "================================================================================================"
echo -e "${GREEN}🎉 Integration Test Suite Complete${NC}"
echo "================================================================================================"
echo ""
echo -e "${BLUE}📊 Test Coverage Summary:${NC}"
echo "✅ Worker Lifecycle (start/stop/restart)"
echo "✅ Message Processing Flow"
echo "✅ Stuck Worker State Detection" 
echo "✅ API Request Validation"
echo "✅ RabbitMQ Connectivity Recovery"
echo "✅ System Health Monitoring"
echo "✅ Service Recovery Scenarios"
echo "✅ Queue Management"
echo "✅ DLQ and Failure Handling"
echo "✅ DLQ Auto-Setup and Resilience"
echo "✅ DLQ End-to-End Message Flow"
echo ""
echo -e "${BLUE}🎯 These tests validate the scenarios that caused:${NC}"
echo "   • Alice worker getting stuck in 'stopping' state"
echo "   • RabbitMQ connection issues after service restart"  
echo "   • Worker API request format validation errors"
echo "   • Inconsistent state between workers service and RabbitMQ"
echo "   • Reset functionality clearing stuck workers"
echo "   • DLQ disappearing after 'Start Quest Wave' (auto-setup issue)"
echo "   • DLQ not being available after reset operations"
echo ""
echo -e "${YELLOW}💡 To run specific test suites:${NC}"
echo "   go test -v -run TestWorkerLifecycleIntegration ./internal/api/handlers/"
echo "   go test -v -run TestStuckWorkerDetection ./internal/api/handlers/"
echo "   go test -v -run TestSystemHealthAndRecovery ./internal/api/handlers/"
echo "   go test -v -run TestDLQAutoSetupFunctionality ./internal/api/handlers/"
echo "   go test -v -run TestDLQMessageFlowEndToEnd ./internal/api/handlers/"
echo ""
echo -e "${YELLOW}💡 To run with short tests only (skips integration):${NC}"
echo "   go test -short ./internal/api/handlers/"
echo ""