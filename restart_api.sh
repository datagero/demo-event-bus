#!/bin/bash

# ==============================================================================
# 🔄 Restart API Server - Development Helper
# ==============================================================================
# This script restarts only the API server while keeping other services running
# It logs the restart action to the main start_app terminal
# ==============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
GO_API_PORT=${GO_API_PORT:-9000}

echo -e "${BLUE}🔄 [DEV] Restarting API Server...${NC}"

# Find and stop the current API server process
echo -e "${YELLOW}🛑 [DEV] Stopping current API server...${NC}"
if pkill -f "api-server-complete" 2>/dev/null; then
    echo -e "${GREEN}✅ [DEV] API server stopped${NC}"
    
    # Wait for the port to be free
    echo -e "${YELLOW}⏳ [DEV] Waiting for port ${GO_API_PORT} to be free...${NC}"
    while lsof -Pi :${GO_API_PORT} -sTCP:LISTEN -t >/dev/null 2>&1; do
        sleep 0.5
    done
    echo -e "${GREEN}✅ [DEV] Port ${GO_API_PORT} is now free${NC}"
else
    echo -e "${YELLOW}⚠️  [DEV] No running API server found${NC}"
fi

# Build and start the new API server
echo -e "${BLUE}🔨 [DEV] Building API server...${NC}"
cd api-server

if go build -o api-server-complete ./main.go; then
    echo -e "${GREEN}✅ [DEV] API server built successfully${NC}"
else
    echo -e "${RED}❌ [DEV] API server build failed${NC}"
    exit 1
fi

echo -e "${BLUE}🚀 [DEV] Starting API server on port ${GO_API_PORT}...${NC}"

# Set environment variables (same as start_app.sh)
export RABBITMQ_URL="amqp://guest:guest@localhost:5672/"
export RABBITMQ_API_URL="http://localhost:15672/api"
export RABBITMQ_USER="guest"
export RABBITMQ_PASS="guest"
export WORKERS_URL="http://localhost:8001"
# PYTHON_URL removed - pure Go implementation

# Start the API server and append to the existing log
./api-server-complete >> ../api-server.log 2>&1 &
GO_API_PID=$!

cd ..

echo -e "${GREEN}✅ [DEV] API server restarted (PID: ${GO_API_PID})${NC}"

# Test if it's responding
echo -e "${BLUE}🧪 [DEV] Testing API server health...${NC}"
for i in {1..10}; do
    if curl -s "http://localhost:${GO_API_PORT}/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ [DEV] API server is responding${NC}"
        echo -e "${GREEN}🎉 [DEV] API server restart completed successfully!${NC}"
        exit 0
    fi
    echo -e "${YELLOW}   [DEV] Waiting for API server... (${i}/10)${NC}"
    sleep 1
done

echo -e "${RED}❌ [DEV] API server health check failed${NC}"
exit 1