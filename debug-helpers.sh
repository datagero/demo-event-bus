#!/bin/bash

# Debug Helper Scripts for Demo Event Bus
# Usage: ./debug-helpers.sh [command]

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}ℹ️  $1${NC}"; }
log_success() { echo -e "${GREEN}✅ $1${NC}"; }
log_warning() { echo -e "${YELLOW}⚠️  $1${NC}"; }
log_error() { echo -e "${RED}❌ $1${NC}"; }

# Quick health check with detailed diagnostics
health_check() {
    log_info "Running comprehensive health check..."
    
    echo "🔍 Service Status:"
    echo "  API Server (9000): $(curl -s http://localhost:9000/health 2>/dev/null | jq -r '.status' || echo 'DOWN')"
    echo "  Workers (8001): $(curl -s http://localhost:8001/health 2>/dev/null | jq -r '.status' || echo 'DOWN')"
    echo "  RabbitMQ (15672): $(curl -s http://localhost:15672/api/overview --user guest:guest 2>/dev/null | jq -r '.rabbitmq_version' || echo 'DOWN')"
    
    echo ""
    echo "🐰 RabbitMQ Quick Stats:"
    curl -s http://localhost:15672/api/queues --user guest:guest 2>/dev/null | jq -r '.[] | select(.name | test("game\\.")) | "  \(.name): \(.messages) pending, \(.consumers) consumers"' || log_error "RabbitMQ API not accessible"
    
    echo ""
    echo "🚀 Running Processes:"
    ps aux | grep -E "(api-server|workers)" | grep -v grep || log_warning "No API/Workers processes found"
}

# Test critical frontend routes that often break
test_frontend_routes() {
    log_info "Testing critical frontend routes..."
    
    local base_url="http://localhost:9000"
    local failed=0
    
    # Test routes that frontend actually calls
    declare -A routes=(
        ["/api/player/start"]="Frontend recruitment (singular)"
        ["/api/players/quickstart"]="Quickstart button"
        ["/api/master/one"]="Send One button"
        ["/api/master/start"]="Start Quest Wave"
        ["/api/rabbitmq/derived/metrics"]="Frontend metrics"
        ["/api/rabbitmq/derived/scoreboard"]="Frontend scoreboard"
        ["/health"]="Health check"
    )
    
    for route in "${!routes[@]}"; do
        local method="GET"
        local payload=""
        
        # Determine HTTP method and payload for each route
        case $route in
            "/api/player/start"|"/api/players/quickstart"|"/api/master/one"|"/api/master/start")
                method="POST"
                payload='{"test": true}'
                ;;
        esac
        
        if [[ $method == "POST" ]]; then
            status=$(curl -s -o /dev/null -w "%{http_code}" -X POST -H "Content-Type: application/json" -d "$payload" "$base_url$route")
        else
            status=$(curl -s -o /dev/null -w "%{http_code}" "$base_url$route")
        fi
        
        if [[ $status =~ ^[2-3][0-9][0-9]$ ]]; then
            log_success "$route (${routes[$route]}) - HTTP $status"
        else
            log_error "$route (${routes[$route]}) - HTTP $status"
            ((failed++))
        fi
    done
    
    if [[ $failed -eq 0 ]]; then
        log_success "All critical routes accessible!"
    else
        log_error "$failed routes failed"
        return 1
    fi
}

# Test recruitment functionality specifically
test_recruitment() {
    log_info "Testing recruitment functionality..."
    
    local base_url="http://localhost:9000"
    
    # Test individual recruitment
    log_info "Testing individual player recruitment..."
    response=$(curl -s -X POST "$base_url/api/player/start" \
        -H "Content-Type: application/json" \
        -d '{"player": "debug_test", "skills": "gather", "fail_pct": 0.1, "speed_multiplier": 1.0, "workers": 1}')
    
    if echo "$response" | jq -e '.ok == true' >/dev/null 2>&1; then
        log_success "Individual recruitment working"
    else
        log_error "Individual recruitment failed: $(echo "$response" | jq -r '.error // "Unknown error"')"
        return 1
    fi
    
    # Test quickstart
    log_info "Testing quickstart recruitment..."
    response=$(curl -s -X POST "$base_url/api/players/quickstart" \
        -H "Content-Type: application/json" \
        -d '{"preset": "alice_bob"}')
    
    if echo "$response" | jq -e '.ok == true' >/dev/null 2>&1; then
        log_success "Quickstart recruitment working"
    else
        log_error "Quickstart recruitment failed: $(echo "$response" | jq -r '.error // "Unknown error"')"
        return 1
    fi
}

# Show route mismatches between frontend and backend
check_route_contracts() {
    log_info "Checking frontend-backend route contracts..."
    
    # Extract routes from frontend
    log_info "Frontend routes called:"
    if [[ -f "legacy/python/app/web_static/index.html" ]]; then
        grep -o "fetch('[^']*" legacy/python/app/web_static/index.html | sed "s/fetch('//" | sort -u | while read route; do
            echo "  $route"
        done
    else
        log_warning "Frontend file not found"
    fi
    
    echo ""
    log_info "Backend routes available:"
    if [[ -f "api-server/internal/api/routes.go" ]]; then
        grep -E "(GET|POST|PUT|DELETE)" api-server/internal/api/routes.go | grep -o '"/[^"]*"' | sort -u | while read route; do
            echo "  $route"
        done
    else
        log_warning "Backend routes file not found"
    fi
}

# Quick test run for changed endpoints
quick_test() {
    log_info "Running quick tests for critical functionality..."
    
    cd api-server
    go test -run "TestPlayerEndpoints|TestMessageEndpoints|TestHealthEndpoint" ./internal/api/handlers/ -v
}

# Restart services in correct order with health checks
restart_services() {
    log_info "Restarting services with health checks..."
    
    # Stop existing services
    log_info "Stopping existing services..."
    pkill -f "api-server" || true
    pkill -f "workers" || true
    sleep 2
    
    # Start API server
    log_info "Starting API server..."
    cd api-server
    if [[ ! -f "api-server-test" ]]; then
        log_info "Building API server..."
        go build -o api-server-test .
    fi
    ./api-server-test > /tmp/api-server.log 2>&1 &
    
    # Wait for API server
    for i in {1..10}; do
        if curl -s http://localhost:9000/health >/dev/null 2>&1; then
            log_success "API server ready"
            break
        fi
        log_info "Waiting for API server... ($i/10)"
        sleep 1
    done
    
    # Start workers
    log_info "Starting workers..."
    cd ../workers
    if [[ ! -f "workers-test" ]]; then
        log_info "Building workers..."
        go build -o workers-test .
    fi
    ./workers-test --port 8001 --webhook "http://localhost:9000/api/go-workers/webhook/events" > /tmp/workers.log 2>&1 &
    
    # Wait for workers
    for i in {1..10}; do
        if curl -s http://localhost:8001/health >/dev/null 2>&1; then
            log_success "Workers ready"
            break
        fi
        log_info "Waiting for workers... ($i/10)"
        sleep 1
    done
    
    cd ..
    log_success "Services restarted successfully"
}

# Show logs from running services
show_logs() {
    log_info "Recent service logs:"
    
    echo ""
    echo "🔧 API Server logs (last 20 lines):"
    tail -20 /tmp/api-server.log 2>/dev/null || log_warning "No API server logs found"
    
    echo ""
    echo "⚡ Workers logs (last 20 lines):"
    tail -20 /tmp/workers.log 2>/dev/null || log_warning "No workers logs found"
}

# Main command dispatcher
case "${1:-help}" in
    "health")
        health_check
        ;;
    "routes")
        test_frontend_routes
        ;;
    "recruitment")
        test_recruitment
        ;;
    "contracts")
        check_route_contracts
        ;;
    "test")
        quick_test
        ;;
    "restart")
        restart_services
        ;;
    "logs")
        show_logs
        ;;
    "full-check")
        health_check
        echo ""
        test_frontend_routes
        echo ""
        test_recruitment
        ;;
    "help"|*)
        echo "🔧 Debug Helper Commands:"
        echo "  ./debug-helpers.sh health       - Quick health check with diagnostics"
        echo "  ./debug-helpers.sh routes       - Test critical frontend routes"
        echo "  ./debug-helpers.sh recruitment  - Test recruitment functionality"
        echo "  ./debug-helpers.sh contracts    - Check frontend-backend route contracts"
        echo "  ./debug-helpers.sh test         - Run quick tests for critical functionality"
        echo "  ./debug-helpers.sh restart      - Restart services with health checks"
        echo "  ./debug-helpers.sh logs         - Show recent service logs"
        echo "  ./debug-helpers.sh full-check   - Run comprehensive diagnostics"
        echo ""
        echo "Example usage:"
        echo "  ./debug-helpers.sh full-check   # Diagnose most issues"
        echo "  ./debug-helpers.sh recruitment  # Debug recruitment problems"
        ;;
esac