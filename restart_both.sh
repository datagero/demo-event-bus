#!/bin/bash

# ==============================================================================
# 🔄 Restart Both Services - Development Helper
# ==============================================================================
# This script restarts both API server and workers while keeping logs unified
# ==============================================================================

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🔄 [DEV] Restarting both API server and workers...${NC}"

# Restart API server first
echo -e "${BLUE}🔄 [DEV] Step 1: Restarting API server...${NC}"
./restart_api.sh

# Small delay to ensure API server is stable
sleep 2

# Restart workers
echo -e "${BLUE}🔄 [DEV] Step 2: Restarting workers...${NC}"
./restart_workers.sh

echo -e "${GREEN}🎉 [DEV] Both services restarted successfully!${NC}"