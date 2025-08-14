#!/bin/bash

# ==============================================================================
# 🔄 Restart Workers - Development Helper
# ==============================================================================
# This script restarts only the workers while keeping other services running
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
GO_WORKERS_PORT=${GO_WORKERS_PORT:-8001}
GO_API_PORT=${GO_API_PORT:-9000}

echo -e "${BLUE}🔄 [DEV] Restarting Workers...${NC}"

# Find and stop the current workers process
echo -e "${YELLOW}🛑 [DEV] Stopping current workers...${NC}"
if pkill -f "workers-complete" 2>/dev/null; then
    echo -e "${GREEN}✅ [DEV] Workers stopped${NC}"
    
    # Wait for the port to be free
    echo -e "${YELLOW}⏳ [DEV] Waiting for port ${GO_WORKERS_PORT} to be free...${NC}"
    while lsof -Pi :${GO_WORKERS_PORT} -sTCP:LISTEN -t >/dev/null 2>&1; do
        sleep 0.5
    done
    echo -e "${GREEN}✅ [DEV] Port ${GO_WORKERS_PORT} is now free${NC}"
else
    echo -e "${YELLOW}⚠️  [DEV] No running workers found${NC}"
fi

# Build and start the new workers
echo -e "${BLUE}🔨 [DEV] Building workers...${NC}"
cd workers

if go build -o workers-complete ./main.go; then
    echo -e "${GREEN}✅ [DEV] Workers built successfully${NC}"
else
    echo -e "${RED}❌ [DEV] Workers build failed${NC}"
    exit 1
fi

echo -e "${BLUE}🚀 [DEV] Starting workers on port ${GO_WORKERS_PORT}...${NC}"

# Start the workers and append to the existing log
./workers-complete --port ${GO_WORKERS_PORT} --webhook "http://localhost:${GO_API_PORT}/api/go-workers/webhook/events" >> ../workers.log 2>&1 &
GO_WORKERS_PID=$!

cd ..

echo -e "${GREEN}✅ [DEV] Workers restarted (PID: ${GO_WORKERS_PID})${NC}"

# Test if it's responding
echo -e "${BLUE}🧪 [DEV] Testing workers health...${NC}"
for i in {1..10}; do
    if curl -s "http://localhost:${GO_WORKERS_PORT}/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ [DEV] Workers are responding${NC}"
        echo -e "${GREEN}🎉 [DEV] Workers restart completed successfully!${NC}"
        exit 0
    fi
    echo -e "${YELLOW}   [DEV] Waiting for workers... (${i}/10)${NC}"
    sleep 1
done

echo -e "${RED}❌ [DEV] Workers health check failed${NC}"
exit 1