#!/bin/bash

# ==============================================================================
# 🚀 Demo Event Bus - Complete Go Migration Startup Script
# ==============================================================================
# This script starts the fully migrated Go-based event bus system
# Features:
# - Go API Server (port 9000) with RabbitMQ-direct architecture  
# - Go Workers (port 8001) for message processing
# - Native RabbitMQ integration with Management API
# - WebSocket support for real-time UI updates
# - Comprehensive DLQ system and chaos engineering
# ==============================================================================

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
MUTED='\033[0;37m'  # Light gray for timestamps
NC='\033[0m' # No Color

# Configuration
RABBITMQ_PORT=${RABBITMQ_PORT:-5672}
RABBITMQ_MGMT_PORT=${RABBITMQ_MGMT_PORT:-15672}
GO_API_PORT=${GO_API_PORT:-9000}
GO_WORKERS_PORT=${GO_WORKERS_PORT:-8001}

# Parse command line arguments
WATCH_MODE=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --watch|-w)
            WATCH_MODE=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  --watch, -w    Enable file watching and auto-restart on Go code changes"
            echo "  --help, -h     Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Check for required tools if watch mode is enabled
if [ "$WATCH_MODE" = true ]; then
    echo -e "${BLUE}🔍 Checking for required tools for watch mode...${NC}"
    
    if ! command -v air >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Air (Go live reload) not found. Installing...${NC}"
        if ! command -v go >/dev/null 2>&1; then
            echo -e "${RED}❌ Go not found. Please install Go first.${NC}"
            exit 1
        fi
        go install github.com/air-verse/air@latest
        
        # Add GOPATH/bin to PATH if not already there
        if [[ ":$PATH:" != *":$(go env GOPATH)/bin:"* ]]; then
            export PATH="$(go env GOPATH)/bin:$PATH"
        fi
    fi
    
    echo -e "${GREEN}✅ Air (Go live reload) ready${NC}"
fi

echo -e "${PURPLE}================================================================================================${NC}"
echo -e "${PURPLE}🚀 Starting Demo Event Bus - Complete Go Migration${NC}"
if [ "$WATCH_MODE" = true ]; then
    echo -e "${YELLOW}📁 Watch Mode: ENABLED - Auto-restart on Go code changes${NC}"
fi
echo -e "${PURPLE}================================================================================================${NC}"
echo -e "${CYAN}📊 Configuration:${NC}"
echo -e "   🐰 RabbitMQ: localhost:${RABBITMQ_PORT} (Management: ${RABBITMQ_MGMT_PORT}) - External"
echo -e "   🏗️  Go API Server: localhost:${GO_API_PORT}"
echo -e "   ⚡ Go Workers: localhost:${GO_WORKERS_PORT}"
if [ "$WATCH_MODE" = true ]; then
    echo -e "   👁️  File Watching: ENABLED"
fi
echo -e "${PURPLE}================================================================================================${NC}"
echo -e "${YELLOW}ℹ️  Note: RabbitMQ must be started manually before running this script${NC}"
echo -e "${YELLOW}   Run: docker-compose up -d${NC}"
echo -e "${PURPLE}================================================================================================${NC}"

# Function to check if a port is in use
check_port() {
    local port=$1
    local service_name=$2
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Port $port is already in use (${service_name}). Continuing...${NC}"
        return 0
    else
        return 1
    fi
}

# Function to wait for service
wait_for_service() {
    local port=$1
    local service_name=$2
    local max_attempts=30
    local attempt=1
    
    echo -e "${BLUE}🔄 Waiting for ${service_name} on port ${port}...${NC}"
    
    while ! nc -z localhost $port 2>/dev/null; do
        if [ $attempt -eq $max_attempts ]; then
            echo -e "${RED}❌ ${service_name} failed to start on port ${port}${NC}"
            return 1
        fi
        echo -e "${YELLOW}   Attempt ${attempt}/${max_attempts}...${NC}"
        sleep 2
        ((attempt++))
    done
    
    echo -e "${GREEN}✅ ${service_name} is ready on port ${port}${NC}"
    return 0
}

# Function to test HTTP endpoint
test_endpoint() {
    local url=$1
    local service_name=$2
    
    echo -e "${BLUE}🧪 Testing ${service_name}: ${url}${NC}"
    
    if curl -s "$url" > /dev/null; then
        echo -e "${GREEN}✅ ${service_name} endpoint is responding${NC}"
        return 0
    else
        echo -e "${RED}❌ ${service_name} endpoint test failed${NC}"
        return 1
    fi
}



# ==============================================================================
# Step 1: Build and Start Go API Server
# ==============================================================================

echo -e "\n${PURPLE}📋 Step 1: Building and Starting Go API Server${NC}"

# Check RabbitMQ availability (but don't manage it)
echo -e "${BLUE}🔍 Checking RabbitMQ availability...${NC}"
if ! nc -z localhost $RABBITMQ_PORT 2>/dev/null; then
    echo -e "${YELLOW}⚠️  RabbitMQ AMQP port $RABBITMQ_PORT not accessible${NC}"
    echo -e "${YELLOW}   Please start RabbitMQ manually: docker-compose up -d${NC}"
    echo -e "${YELLOW}   Continuing anyway - services will retry connections...${NC}"
else
    echo -e "${GREEN}✅ RabbitMQ AMQP port $RABBITMQ_PORT is accessible${NC}"
fi

cd api-server

# Set environment variables for Go API server
export RABBITMQ_URL="amqp://guest:guest@localhost:5672/"
export RABBITMQ_API_URL="http://localhost:15672/api"
export RABBITMQ_USER="guest"
export RABBITMQ_PASS="guest"
export WORKERS_URL="http://localhost:${GO_WORKERS_PORT}"
export PYTHON_URL="http://localhost:8080"  # Legacy fallback (not used)

if [ "$WATCH_MODE" = true ]; then
    echo -e "${BLUE}🚀 Starting Go API Server with Air (live reload) on port ${GO_API_PORT}...${NC}"
    mkdir -p tmp
    air > ../api-server.log 2>&1 &
    GO_API_PID=$!
    echo -e "${GREEN}✅ Go API Server started with Air (PID: ${GO_API_PID}), logging to api-server.log${NC}"
else
    echo -e "${BLUE}🔨 Building Go API Server...${NC}"
    if go build -o api-server-complete ./main.go; then
        echo -e "${GREEN}✅ Go API Server built successfully${NC}"
    else
        echo -e "${RED}❌ Go API Server build failed${NC}"
        exit 1
    fi

    echo -e "${BLUE}🚀 Starting Go API Server on port ${GO_API_PORT}...${NC}"
    ./api-server-complete > ../api-server.log 2>&1 &
    GO_API_PID=$!
    echo -e "${GREEN}✅ Go API Server started (PID: ${GO_API_PID}), logging to api-server.log${NC}"
fi

wait_for_service $GO_API_PORT "Go API Server"
test_endpoint "http://localhost:${GO_API_PORT}/health" "Go API Server Health"

cd ..

# ==============================================================================
# Step 2: Build and Start Go Workers
# ==============================================================================

echo -e "\n${PURPLE}📋 Step 2: Building and Starting Go Workers${NC}"

cd workers

if [ "$WATCH_MODE" = true ]; then
    echo -e "${BLUE}🚀 Starting Go Workers with Air (live reload) on port ${GO_WORKERS_PORT}...${NC}"
    mkdir -p tmp
    air > ../workers.log 2>&1 &
    GO_WORKERS_PID=$!
    echo -e "${GREEN}✅ Go Workers started with Air (PID: ${GO_WORKERS_PID}), logging to workers.log${NC}"
else
    echo -e "${BLUE}🔨 Building Go Workers...${NC}"
    if go build -o workers-complete ./main.go; then
        echo -e "${GREEN}✅ Go Workers built successfully${NC}"
    else
        echo -e "${RED}❌ Go Workers build failed${NC}"
        exit 1
    fi

    echo -e "${BLUE}🚀 Starting Go Workers on port ${GO_WORKERS_PORT}...${NC}"
    ./workers-complete --port ${GO_WORKERS_PORT} --webhook "http://localhost:${GO_API_PORT}/api/go-workers/webhook/events" > ../workers.log 2>&1 &
    GO_WORKERS_PID=$!
    echo -e "${GREEN}✅ Go Workers started (PID: ${GO_WORKERS_PID}), logging to workers.log${NC}"
fi

wait_for_service $GO_WORKERS_PORT "Go Workers"
test_endpoint "http://localhost:${GO_WORKERS_PORT}/health" "Go Workers Health"

cd ..

# ==============================================================================
# Step 3: Test Complete System Integration
# ==============================================================================

echo -e "\n${PURPLE}📋 Step 3: Testing Complete System Integration${NC}"

echo -e "${BLUE}🧪 Testing Go API Server endpoints...${NC}"

# Test RabbitMQ metrics (RabbitMQ-direct)
if curl -s "http://localhost:${GO_API_PORT}/api/rabbitmq/metrics" | grep -q "direct_rabbitmq_go_client"; then
    echo -e "${GREEN}✅ RabbitMQ-direct metrics working${NC}"
else
    echo -e "${RED}❌ RabbitMQ-direct metrics test failed${NC}"
fi

# Test chaos status
if curl -s "http://localhost:${GO_API_PORT}/api/chaos/status" | grep -q "rabbitmq_native"; then
    echo -e "${GREEN}✅ Chaos engineering endpoints working${NC}"
else
    echo -e "${RED}❌ Chaos engineering test failed${NC}"
fi

# Test message publishing
echo -e "${BLUE}🧪 Testing native message publishing...${NC}"
PUBLISH_RESULT=$(curl -s -X POST "http://localhost:${GO_API_PORT}/api/publish" \
  -H "Content-Type: application/json" \
  -d '{"routing_key":"game.quest.gather","payload":{"case_id":"startup-test","quest_type":"gather","points":5}}')

if echo "$PUBLISH_RESULT" | grep -q "go_api_server"; then
    echo -e "${GREEN}✅ Native Go message publishing working${NC}"
else
    echo -e "${RED}❌ Message publishing test failed${NC}"
fi

# ==============================================================================
# Step 4: Display System Information
# ==============================================================================

echo -e "\n${PURPLE}📋 Step 4: System Information${NC}"

echo -e "\n${CYAN}🌐 Service URLs:${NC}"
echo -e "   🏠 Frontend:              http://localhost:${GO_API_PORT}/"
echo -e "   🔧 Go API Server:         http://localhost:${GO_API_PORT}/api/"
echo -e "   ⚡ Go Workers:            http://localhost:${GO_WORKERS_PORT}/"
echo -e "   🐰 RabbitMQ Management:   http://localhost:${RABBITMQ_MGMT_PORT}/ (guest/guest)"

echo -e "\n${CYAN}🔗 Key Endpoints:${NC}"
echo -e "   📊 RabbitMQ Metrics:      http://localhost:${GO_API_PORT}/api/rabbitmq/metrics"
echo -e "   👥 Player Creation:       POST http://localhost:${GO_API_PORT}/api/players/quickstart"
echo -e "   📨 Message Publishing:    POST http://localhost:${GO_API_PORT}/api/publish"
echo -e "   ⚡ Chaos Engineering:     POST http://localhost:${GO_API_PORT}/api/chaos/arm"
echo -e "   🎮 Scenarios:             POST http://localhost:${GO_API_PORT}/api/scenario/run"
echo -e "   💀 DLQ Management:        POST http://localhost:${GO_API_PORT}/api/dlq/setup"

echo -e "\n${CYAN}📈 Educational Features:${NC}"
echo -e "   🎯 RabbitMQ-Direct Architecture - Zero abstraction layers"
echo -e "   🔍 Management API Integration - Live broker introspection"
echo -e "   ⚡ Native Chaos Engineering - Direct RabbitMQ operations"
echo -e "   💀 Comprehensive DLQ System - Native dead letter handling"
echo -e "   🌐 WebSocket Broadcasting - Real-time UI updates"
echo -e "   🚀 Pure Go Implementation - No Python dependencies"

echo -e "\n${CYAN}🎮 Quick Start Commands:${NC}"
echo -e "   # Create Alice and Bob workers"
echo -e "   curl -X POST http://localhost:${GO_API_PORT}/api/players/quickstart -H 'Content-Type: application/json' -d '{\"preset\":\"alice_bob\"}'"
echo -e ""
echo -e "   # Publish a quest"
echo -e "   curl -X POST http://localhost:${GO_API_PORT}/api/publish -H 'Content-Type: application/json' -d '{\"routing_key\":\"game.quest.gather\",\"payload\":{\"case_id\":\"quest-1\",\"quest_type\":\"gather\",\"points\":5}}'"
echo -e ""
echo -e "   # Run late-bind escort scenario"
echo -e "   curl -X POST http://localhost:${GO_API_PORT}/api/scenario/run -H 'Content-Type: application/json' -d '{\"scenario\":\"late-bind-escort\"}'"
echo -e ""
echo -e "   # Trigger chaos (purge queue)"
echo -e "   curl -X POST http://localhost:${GO_API_PORT}/api/chaos/arm -H 'Content-Type: application/json' -d '{\"action\":\"rmq_purge_queue\",\"target_queue\":\"game.skill.gather.q\"}'"

if [ "$WATCH_MODE" = true ]; then
    echo -e ""
    echo -e "${CYAN}👁️  Development Mode:${NC}"
    echo -e "   📁 Air live reload is ENABLED"
    echo -e "   🔄 Go files will auto-rebuild and restart on changes"
    echo -e "   📝 Edit files in api-server/ or workers/ to see instant updates"
fi

# ==============================================================================
# Cleanup Function
# ==============================================================================

cleanup() {
    echo -e "\n${YELLOW}🛑 Shutting down services...${NC}"
    
    if [ ! -z "$GO_WORKERS_PID" ]; then
        echo -e "${BLUE}Stopping Go Workers (PID: ${GO_WORKERS_PID})...${NC}"
        kill $GO_WORKERS_PID 2>/dev/null || true
    fi
    
    if [ ! -z "$GO_API_PID" ]; then
        echo -e "${BLUE}Stopping Go API Server (PID: ${GO_API_PID})...${NC}"
        kill $GO_API_PID 2>/dev/null || true
    fi
    
    echo -e "${GREEN}✅ All services stopped${NC}"
}

# Set up cleanup on script exit
trap cleanup EXIT

# ==============================================================================
# Keep Running
# ==============================================================================

echo -e "\n${PURPLE}================================================================================================${NC}"
echo -e "${GREEN}🎉 Demo Event Bus - Complete Go Migration is running!${NC}"
echo -e "${PURPLE}================================================================================================${NC}"
echo -e "${CYAN}📊 Architecture: RabbitMQ (External) + Go API Server + Go Workers${NC}"
echo -e "${CYAN}🎯 Educational Focus: Direct RabbitMQ integration with minimal abstraction${NC}"
echo -e "${CYAN}🌐 Frontend: http://localhost:${GO_API_PORT}/${NC}"
echo -e "${YELLOW}ℹ️  RabbitMQ managed separately via: docker-compose up -d${NC}"
echo -e "${PURPLE}================================================================================================${NC}"
echo -e "${YELLOW}Press Ctrl+C to stop all services${NC}"

# Keep running until manually stopped - show logs from both services
echo -e "\n${BLUE}🔄 Monitoring services... (Check status with: ./check_status.sh)${NC}"
echo -e "${BLUE}🔄 To restart individual services during development:${NC}"
echo -e "${BLUE}   ./restart_api.sh     - Restart API server${NC}"
echo -e "${BLUE}   ./restart_workers.sh - Restart workers${NC}"
echo -e "${PURPLE}================================================================================================${NC}"
echo -e "${CYAN}📋 Live Logs (API Server + Workers):${NC}"
echo -e "${MUTED}Tip: Use Ctrl+C to stop all services${NC}"
echo -e "${PURPLE}================================================================================================${NC}"

# Follow both log files in real-time with source prefixes and enhanced filtering
tail -f api-server.log workers.log | while IFS= read -r line; do
    current_source=""
    
    # Detect which log file the line came from
    if [[ "$line" == "==> api-server.log <==" ]]; then
        current_source="API_SERVER"
        continue
    elif [[ "$line" == "==> workers.log <==" ]]; then
        current_source="WORKERS"
        continue
    elif [[ "$line" != "" && "$line" != "==> "* ]]; then
        # Enhanced filtering - skip noisy/refresh logs but keep relevant ones
        if [[ "$line" =~ (GET.*derived/metrics|GET.*derived/scoreboard|GET.*"/"|GET.*"/"$|📊.*Derived\ metrics|📋.*\[DLQ\].*attempting\ to\ peek|📋.*\[DLQ\].*retrieved.*messages) ]]; then
            continue # Skip refresh/home page requests, repetitive metrics, and DLQ polling logs
        fi
        
        # Keep relevant API server logs: DLQ operations (setup/reissue), publishing, worker management, etc.
        if [[ "$line" =~ (correlation_id|POST.*publish|POST.*workers|📤.*Published|⚰️.*DLQ|🔄.*DLQ.*setup|❌.*DLQ|✅.*DLQ.*setup|DLQ.*reissue) ]]; then
            echo -e "${MUTED}$(date '+%H:%M:%S')${NC} ${BLUE}[API_SERVER]${NC} $line"
        # Keep worker logs: worker lifecycle, message processing, etc.
        elif [[ "$line" =~ (⚡.*\[WORKERS\]|Worker.*started|Worker.*stopped|Processing.*message|🔧.*Worker) ]]; then
            echo -e "${MUTED}$(date '+%H:%M:%S')${NC} ${GREEN}[WORKERS]${NC} $line"
        # Keep GIN logs that are not refresh requests (POST, PUT, DELETE, non-root GET)
        elif [[ "$line" =~ \[GIN\] && ! "$line" =~ (GET.*"/"$|GET.*derived) ]]; then
            echo -e "${MUTED}$(date '+%H:%M:%S')${NC} ${BLUE}[API_SERVER]${NC} $line"
        # Keep other significant logs (errors, startup, etc.)
        elif [[ "$line" =~ (ERROR|WARN|Starting|Stopping|Failed|Success|✅|❌|🚀|🛑) ]]; then
            # Determine source based on context
            if [[ "$line" =~ (api-server|API|gin|GIN|Port.*9000) ]]; then
                echo -e "${MUTED}$(date '+%H:%M:%S')${NC} ${BLUE}[API_SERVER]${NC} $line"
            elif [[ "$line" =~ (workers|worker|Worker|Port.*8001) ]]; then
                echo -e "${MUTED}$(date '+%H:%M:%S')${NC} ${GREEN}[WORKERS]${NC} $line"
            else
                echo -e "${MUTED}$(date '+%H:%M:%S')${NC} ${PURPLE}[SYSTEM]${NC} $line"
            fi
        fi
    fi
done