#!/bin/bash

# API Server Test Runner
# Provides organized testing for the API server with clear separation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
TIMEOUT=${TIMEOUT:-60s}
VERBOSE=${VERBOSE:-false}
COVERAGE=${COVERAGE:-false}

# Help function
show_help() {
    echo "API Server Test Runner"
    echo ""
    echo "Usage: $0 [OPTIONS] [TEST_TYPE]"
    echo ""
    echo "Options:"
    echo "  -h, --help     Show this help message"
    echo "  -v, --verbose  Run tests in verbose mode"
    echo "  -c, --coverage Enable coverage reporting"
    echo "  -t, --timeout  Set test timeout (default: 60s)"
    echo ""
    echo "Test Types:"
    echo "  unit           Run only unit tests (fast)"
    echo "  integration    Run only integration tests"
    echo "  scenarios      Run only scenario tests"
    echo "  all            Run all tests (default)"
    echo ""
    echo "Examples:"
    echo "  $0 unit                    # Run unit tests"
    echo "  $0 -v integration         # Run integration tests with verbose output"
    echo "  $0 -c all                 # Run all tests with coverage"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -c|--coverage)
            COVERAGE=true
            shift
            ;;
        -t|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        unit|integration|scenarios|all)
            TEST_TYPE="$1"
            shift
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Default to all tests if no type specified
TEST_TYPE=${TEST_TYPE:-all}

echo -e "${BLUE}🧪 API Server Test Runner${NC}"
echo "=========================="
echo ""
echo "Configuration:"
echo "  Test Type: $TEST_TYPE"
echo "  Verbose: $VERBOSE"
echo "  Coverage: $COVERAGE"
echo "  Timeout: $TIMEOUT"
echo ""

# Build test flags
TEST_FLAGS=""
if [ "$VERBOSE" = true ]; then
    TEST_FLAGS="$TEST_FLAGS -v"
fi
if [ "$COVERAGE" = true ]; then
    TEST_FLAGS="$TEST_FLAGS -cover"
fi
TEST_FLAGS="$TEST_FLAGS -timeout $TIMEOUT"

# Function to run specific test type
run_test_type() {
    local type=$1
    local description=$2
    local path=$3
    
    echo -e "${BLUE}📋 Running: $description${NC}"
    echo "   Directory: $path"
    echo "   ----------------------------------------"
    
    if [ -d "tests/$path" ] && [ "$(ls -A tests/$path/*.go 2>/dev/null)" ]; then
        if go test $TEST_FLAGS ./tests/$path/...; then
            echo -e "   ${GREEN}✅ $description: PASSED${NC}"
            return 0
        else
            echo -e "   ${RED}❌ $description: FAILED${NC}"
            return 1
        fi
    else
        echo -e "   ${YELLOW}⚠️  No tests found - SKIPPED${NC}"
        return 0
    fi
    echo ""
}

# Track results
total=0
passed=0
failed=0

# Run tests based on type
case $TEST_TYPE in
    unit)
        echo -e "${GREEN}🚀 Running Unit Tests${NC}"
        echo "====================="
        echo ""
        total=$((total + 1))
        if run_test_type "unit" "Unit Tests" "unit"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
        fi
        ;;
    integration)
        echo -e "${GREEN}🚀 Running Integration Tests${NC}"
        echo "============================="
        echo ""
        total=$((total + 1))
        if run_test_type "integration" "Integration Tests" "integration"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
        fi
        ;;
    scenarios)
        echo -e "${GREEN}🚀 Running Scenario Tests${NC}"
        echo "========================="
        echo ""
        total=$((total + 1))
        if run_test_type "scenarios" "Scenario Tests" "scenarios"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
        fi
        ;;
    all)
        echo -e "${GREEN}🚀 Running All Tests${NC}"
        echo "===================="
        echo ""
        
        # Unit tests
        total=$((total + 1))
        if run_test_type "unit" "Unit Tests" "unit"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
        fi
        
        # Integration tests  
        total=$((total + 1))
        if run_test_type "integration" "Integration Tests" "integration"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
        fi
        
        # Scenario tests
        total=$((total + 1))
        if run_test_type "scenarios" "Scenario Tests" "scenarios"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
        fi
        ;;
esac

# Summary
echo ""
echo -e "${BLUE}📊 Test Results Summary${NC}"
echo "======================="
echo "Total Test Suites: $total"
echo "Passed: $passed"
echo "Failed: $failed"
echo ""

if [ $failed -eq 0 ]; then
    echo -e "${GREEN}🎉 All tests passed!${NC}"
    echo "The API server is working correctly."
    exit 0
else
    echo -e "${RED}❌ Some tests failed${NC}"
    echo "Please review the failures above."
    exit 1
fi