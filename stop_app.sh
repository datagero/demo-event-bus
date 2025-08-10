#!/bin/bash

# ==============================================================================
# 🛑 Demo Event Bus - Stop Script
# ==============================================================================
# Stops all services and cleans up processes
# ==============================================================================

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

echo -e "${PURPLE}================================================================================================${NC}"
echo -e "${PURPLE}🛑 Stopping Demo Event Bus Services${NC}"
echo -e "${PURPLE}================================================================================================${NC}"

# Function to stop processes by name
stop_processes() {
    local process_name=$1
    local display_name=$2
    
    echo -e "${BLUE}🔄 Stopping ${display_name}...${NC}"
    
    # Find and kill processes
    local pids=$(pgrep -f "$process_name" 2>/dev/null || true)
    
    if [ -n "$pids" ]; then
        echo "$pids" | xargs kill -TERM 2>/dev/null || true
        sleep 2
        
        # Force kill if still running
        local remaining_pids=$(pgrep -f "$process_name" 2>/dev/null || true)
        if [ -n "$remaining_pids" ]; then
            echo "$remaining_pids" | xargs kill -KILL 2>/dev/null || true
        fi
        
        echo -e "${GREEN}✅ ${display_name} stopped${NC}"
    else
        echo -e "${YELLOW}⚠️  ${display_name} not running${NC}"
    fi
}

# ==============================================================================
# Stop Go Services
# ==============================================================================

echo -e "\n${PURPLE}📋 Stopping Go Services${NC}"

stop_processes "api-server-complete" "Go API Server"
stop_processes "workers-complete" "Go Workers"
stop_processes "api-server-test" "Go API Server (test)"
stop_processes "start_app.sh" "Go Startup Script"

# ==============================================================================
# Stop Legacy Python Services (if any)
# ==============================================================================

echo -e "\n${PURPLE}📋 Stopping Legacy Services${NC}"

stop_processes "web_server.py" "Python Web Server"
stop_processes "uvicorn" "Python Uvicorn"
stop_processes "start_app.sh" "Legacy Startup Scripts"

# ==============================================================================
# Stop RabbitMQ
# ==============================================================================

echo -e "\n${PURPLE}📋 Stopping RabbitMQ${NC}"

if command -v docker-compose &> /dev/null; then
    echo -e "${BLUE}🔄 Stopping RabbitMQ via Docker Compose...${NC}"
    docker-compose down 2>/dev/null || true
    echo -e "${GREEN}✅ RabbitMQ stopped${NC}"
else
    echo -e "${YELLOW}⚠️  Docker Compose not found, skipping RabbitMQ stop${NC}"
fi

# ==============================================================================
# Clean Up Temporary Files
# ==============================================================================

echo -e "\n${PURPLE}📋 Cleaning Up Temporary Files${NC}"

# Remove built binaries
if [ -f "api-server/api-server-complete" ]; then
    rm "api-server/api-server-complete"
    echo -e "${GREEN}✅ Removed api-server-complete binary${NC}"
fi

if [ -f "api-server/api-server-test" ]; then
    rm "api-server/api-server-test"
    echo -e "${GREEN}✅ Removed api-server-test binary${NC}"
fi

if [ -f "workers/workers-complete" ]; then
    rm "workers/workers-complete"
    echo -e "${GREEN}✅ Removed workers-complete binary${NC}"
fi

# Clean up Python cache (if any)
if [ -d "__pycache__" ]; then
    rm -rf __pycache__
    echo -e "${GREEN}✅ Removed Python cache${NC}"
fi

if [ -d "app/__pycache__" ]; then
    rm -rf "app/__pycache__"
    echo -e "${GREEN}✅ Removed app Python cache${NC}"
fi

# Clean up any .pyc files
find . -name "*.pyc" -delete 2>/dev/null || true

# ==============================================================================
# Verify Clean State
# ==============================================================================

echo -e "\n${PURPLE}📋 Verifying Clean State${NC}"

# Check for running processes
check_process() {
    local process_name=$1
    local display_name=$2
    
    if pgrep -f "$process_name" > /dev/null 2>&1; then
        echo -e "${RED}❌ ${display_name} still running${NC}"
        return 1
    else
        echo -e "${GREEN}✅ ${display_name} stopped${NC}"
        return 0
    fi
}

all_clean=true

check_process "api-server" "Go API Server" || all_clean=false
check_process "workers.*main.go" "Go Workers" || all_clean=false
check_process "web_server.py" "Python Web Server" || all_clean=false
check_process "uvicorn" "Python Uvicorn" || all_clean=false

# Check Docker containers
if docker ps --format "table {{.Names}}" 2>/dev/null | grep -q "rabbitmq"; then
    echo -e "${RED}❌ RabbitMQ container still running${NC}"
    all_clean=false
else
    echo -e "${GREEN}✅ RabbitMQ container stopped${NC}"
fi

# ==============================================================================
# Summary
# ==============================================================================

echo -e "\n${PURPLE}================================================================================================${NC}"

if [ "$all_clean" = true ]; then
    echo -e "${GREEN}🎉 All Demo Event Bus services stopped successfully!${NC}"
    echo -e "${PURPLE}================================================================================================${NC}"
    echo -e "${CYAN}📊 Clean State Achieved:${NC}"
    echo -e "   ✅ Go API Server stopped"
    echo -e "   ✅ Go Workers stopped"
    echo -e "   ✅ RabbitMQ stopped"
    echo -e "   ✅ Legacy services stopped"
    echo -e "   ✅ Temporary files cleaned"
    echo -e "${PURPLE}================================================================================================${NC}"
    echo -e "${GREEN}Ready to start fresh with: ${YELLOW}./start_app.sh${NC}"
else
    echo -e "${YELLOW}⚠️  Some services may still be running. Check manually if needed.${NC}"
    echo -e "${PURPLE}================================================================================================${NC}"
    echo -e "${CYAN}Manual cleanup commands:${NC}"
    echo -e "   ${YELLOW}pkill -f api-server${NC}"
    echo -e "   ${YELLOW}pkill -f workers${NC}"
    echo -e "   ${YELLOW}docker-compose down${NC}"
    echo -e "${PURPLE}================================================================================================${NC}"
fi

echo -e "${BLUE}🔄 To start the system again: ${YELLOW}./start_app.sh${NC}"