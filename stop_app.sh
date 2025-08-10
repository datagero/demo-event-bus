#!/bin/bash

# Demo Event Bus - Shutdown Script
# Stops all services and cleans up cache

set -e  # Exit on any error

echo "🛑 Stopping Demo Event Bus System"
echo "================================="

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to print colored messages
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Step 1: Stop Python web server
print_status "Stopping Python web server..."
if [ -f ".web_server.pid" ]; then
    WEB_PID=$(cat .web_server.pid)
    if kill $WEB_PID 2>/dev/null; then
        print_success "Web server stopped (PID: $WEB_PID)"
    else
        print_warning "Web server process not found (PID: $WEB_PID)"
    fi
    rm -f .web_server.pid
else
    # Fallback: kill by process name
    pkill -f "uvicorn.*8000" 2>/dev/null && print_success "Web server stopped (by name)" || print_warning "No web server process found"
fi

# Step 2: Stop Go backend
print_status "Stopping Go backend..."
if [ -f ".go_backend.pid" ]; then
    GO_PID=$(cat .go_backend.pid)
    if kill $GO_PID 2>/dev/null; then
        print_success "Go backend stopped (PID: $GO_PID)"
    else
        print_warning "Go backend process not found (PID: $GO_PID)"
    fi
    rm -f .go_backend.pid
else
    # Fallback: kill by process name (including air processes)
    pkill -f "go run main.go" 2>/dev/null && print_success "Go backend stopped (by name)" || print_warning "No Go backend process found"
fi

# Stop air hot reload processes
pkill -f "air" 2>/dev/null && print_success "Air hot reload stopped" || true

# Stop tmux sessions for development modes
if command -v tmux >/dev/null 2>&1; then
    tmux kill-session -t demo-event-bus 2>/dev/null && print_success "Tmux session stopped" || true
fi

# Step 3: Stop RabbitMQ container
print_status "Stopping RabbitMQ container..."
if command -v rancher >/dev/null 2>&1; then
    print_status "Using Rancher for container management..."
    rancher compose down rabbitmq 2>/dev/null && print_success "RabbitMQ container stopped" || print_warning "RabbitMQ container may not be running"
elif command -v docker >/dev/null 2>&1; then
    print_status "Using Docker Compose..."
    docker compose down rabbitmq 2>/dev/null && print_success "RabbitMQ container stopped" || print_warning "RabbitMQ container may not be running"
else
    print_warning "Neither Rancher nor Docker found. Manual container cleanup may be needed."
fi

# Step 4: Clean up cache and temporary files
print_status "Cleaning up cache and temporary files..."

# Remove game state cache
if [ -f ".game_state_cache.pkl" ]; then
    rm -f .game_state_cache.pkl
    print_success "Removed game state cache (.game_state_cache.pkl)"
fi

# Remove log files
if [ -f "go-workers.log" ]; then
    rm -f go-workers.log
    print_success "Removed Go backend logs"
fi

if [ -f "web-server.log" ]; then
    rm -f web-server.log
    print_success "Removed web server logs"
fi

# Remove any other temporary files
rm -f .web_server.pid .go_backend.pid 2>/dev/null || true

# Optional: Clean up Python cache
if [ -d "__pycache__" ]; then
    rm -rf __pycache__
    print_success "Removed Python cache"
fi

if [ -d "app/__pycache__" ]; then
    rm -rf app/__pycache__
    print_success "Removed app Python cache"
fi

# Step 5: Force cleanup any remaining processes
print_status "Performing final cleanup..."

# Kill any remaining processes on the ports
lsof -ti :8000 2>/dev/null | xargs kill -9 2>/dev/null || true
lsof -ti :8001 2>/dev/null | xargs kill -9 2>/dev/null || true

# Step 6: Verify shutdown
print_status "Verifying shutdown..."

WEB_CHECK=$(curl -s http://localhost:8000 >/dev/null 2>&1 && echo "❌ Still running" || echo "✅ Stopped")
GO_CHECK=$(curl -s http://localhost:8001 >/dev/null 2>&1 && echo "❌ Still running" || echo "✅ Stopped")
RABBIT_CHECK=$(curl -s http://localhost:15672 >/dev/null 2>&1 && echo "❌ Still running" || echo "✅ Stopped")

echo ""
echo "🎯 SHUTDOWN STATUS"
echo "=================="
echo "Web Server:   $WEB_CHECK"
echo "Go Backend:   $GO_CHECK"
echo "RabbitMQ:     $RABBIT_CHECK"
echo ""

# Check if everything is stopped
if [[ "$WEB_CHECK" == *"Stopped"* ]] && [[ "$GO_CHECK" == *"Stopped"* ]] && [[ "$RABBIT_CHECK" == *"Stopped"* ]]; then
    print_success "All services stopped successfully!"
    echo ""
    echo "🧹 CLEANUP COMPLETED:"
    echo "• Game state cache cleared"
    echo "• Log files removed"
    echo "• Temporary files cleaned"
    echo "• All processes terminated"
    echo ""
    echo "🚀 Run ./start_app.sh to restart the system"
else
    print_warning "Some services may still be running. Check manually if needed."
    if [[ "$WEB_CHECK" == *"Still running"* ]]; then
        echo "  • Web server may need manual stop: sudo lsof -ti :8000 | xargs kill -9"
    fi
    if [[ "$GO_CHECK" == *"Still running"* ]]; then
        echo "  • Go backend may need manual stop: sudo lsof -ti :8001 | xargs kill -9"
    fi
    if [[ "$RABBIT_CHECK" == *"Still running"* ]]; then
        echo "  • RabbitMQ may need manual stop: docker/rancher compose down"
    fi
fi

echo ""
echo "✅ Shutdown script completed"