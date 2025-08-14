#!/bin/bash

# ==============================================================================
# 📊 Check Service Status - Development Helper
# ==============================================================================
# This script checks the status of all services quickly
# ==============================================================================

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
GO_API_PORT=${GO_API_PORT:-9000}
GO_WORKERS_PORT=${GO_WORKERS_PORT:-8001}
RABBITMQ_PORT=${RABBITMQ_PORT:-5672}
RABBITMQ_MGMT_PORT=${RABBITMQ_MGMT_PORT:-15672}

echo -e "${BLUE}📊 [STATUS] Checking service status...${NC}"

# Check API Server
echo -n -e "${BLUE}🔧 API Server (${GO_API_PORT}): ${NC}"
if curl -s "http://localhost:${GO_API_PORT}/health" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Running${NC}"
else
    echo -e "${RED}❌ Down${NC}"
fi

# Check Workers
echo -n -e "${BLUE}⚡ Workers (${GO_WORKERS_PORT}): ${NC}"
if curl -s "http://localhost:${GO_WORKERS_PORT}/health" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Running${NC}"
else
    echo -e "${RED}❌ Down${NC}"
fi

# Check RabbitMQ AMQP
echo -n -e "${BLUE}🐰 RabbitMQ AMQP (${RABBITMQ_PORT}): ${NC}"
if nc -z localhost ${RABBITMQ_PORT} 2>/dev/null; then
    echo -e "${GREEN}✅ Running${NC}"
else
    echo -e "${RED}❌ Down${NC}"
fi

# Check RabbitMQ Management
echo -n -e "${BLUE}🐰 RabbitMQ Management (${RABBITMQ_MGMT_PORT}): ${NC}"
if curl -s "http://localhost:${RABBITMQ_MGMT_PORT}/api/overview" -u guest:guest > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Running${NC}"
else
    echo -e "${RED}❌ Down${NC}"
fi

# Check processes
echo -e "\n${BLUE}📋 [STATUS] Process information:${NC}"
API_PID=$(pgrep -f "api-server-complete" || echo "Not found")
WORKERS_PID=$(pgrep -f "workers-complete" || echo "Not found")

echo -e "${BLUE}🔧 API Server PID: ${NC}${API_PID}"
echo -e "${BLUE}⚡ Workers PID: ${NC}${WORKERS_PID}"

echo -e "\n${BLUE}📊 [STATUS] Quick health check completed${NC}"