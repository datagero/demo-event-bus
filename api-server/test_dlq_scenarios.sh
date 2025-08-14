#!/bin/bash

# DLQ Scenario Test Runner
# Tests DLQ auto-setup, resilience, and message flow scenarios

set -e

echo "🧪 DLQ Scenario Test Suite"
echo "========================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to run a test suite with error handling
run_test_suite() {
    local test_name="$1"
    local test_pattern="$2"
    local description="$3"
    
    echo ""
    echo -e "${BLUE}📋 Running: $test_name${NC}"
    echo -e "${YELLOW}Description: $description${NC}"
    echo "----------------------------------------"
    
    if go test -v -run "$test_pattern" ./internal/api/handlers/; then
        echo -e "${GREEN}✅ $test_name: PASSED${NC}"
        return 0
    else
        echo -e "${RED}❌ $test_name: FAILED${NC}"
        return 1
    fi
}

# Check if services are running
echo -e "${BLUE}🔍 Checking service availability...${NC}"

# Check RabbitMQ
if ! curl -s http://localhost:15672/api/overview -u guest:guest > /dev/null 2>&1; then
    echo -e "${RED}❌ RabbitMQ Management API not available on localhost:15672${NC}"
    echo "Please start RabbitMQ before running DLQ tests"
    exit 1
fi

# Check API Server
if ! curl -s http://localhost:9000/api/health > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠️ API Server not available on localhost:9000${NC}"
    echo "Some integration tests may be skipped"
fi

# Check Workers Service
if ! curl -s http://localhost:8001/status > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠️ Workers Service not available on localhost:8001${NC}"
    echo "Worker-dependent tests may be skipped"
fi

echo -e "${GREEN}✅ Service availability check complete${NC}"

# Change to api-server directory for Go tests  
cd "$(dirname "$0")" || { echo "❌ Failed to change to script directory"; exit 1; }

# Test execution tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Run test suites
echo ""
echo -e "${BLUE}🚀 Starting DLQ Test Execution${NC}"
echo "================================"

# 1. Unit Tests (no external dependencies)
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test_suite "DLQ Unit Tests" "TestDLQHelperFunctions|TestDLQTopologyValidation" "Unit tests for DLQ helper functions and validation"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 2. Auto-Setup Integration Tests
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test_suite "DLQ Auto-Setup Tests" "TestDLQAutoSetupFunctionality" "Tests DLQ auto-setup when queues don't exist"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 3. Resilience Tests (reset recovery)
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test_suite "DLQ Resilience Tests" "TestDLQResilienceAfterReset" "Tests DLQ recovery after reset operations"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 4. End-to-End Message Flow Tests
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test_suite "DLQ Message Flow Tests" "TestDLQMessageFlowEndToEnd|TestDLQMessageReissue" "Tests complete DLQ message flow scenarios"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# Summary
echo ""
echo "📊 Test Execution Summary"
echo "========================"
echo -e "Total Test Suites: $TOTAL_TESTS"
echo -e "${GREEN}Passed: $PASSED_TESTS${NC}"
echo -e "${RED}Failed: $FAILED_TESTS${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo ""
    echo -e "${GREEN}🎉 All DLQ test suites passed!${NC}"
    echo -e "${BLUE}The DLQ auto-setup and resilience features are working correctly.${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}⚠️ Some test suites failed.${NC}"
    echo -e "${YELLOW}Check the output above for details.${NC}"
    exit 1
fi