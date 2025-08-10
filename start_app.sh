#!/bin/bash

# Demo Event Bus - Startup Script
# Initializes containers, Go backend, and Python web server
# Usage: ./start_app.sh [mode]
#   mode: normal (default) | hotreload | dev

set -e  # Exit on any error

# Parse command line arguments
MODE="${1:-normal}"

case $MODE in
    normal)
        echo "🚀 Starting Demo Event Bus System"
        echo "================================="
        ;;
    hotreload)
        echo "🔥 Starting Demo Event Bus System (Hot Reload Mode)"
        echo "=================================================="
        ;;
    dev)
        echo "🚀 Starting Demo Event Bus System (Development Mode)"
        echo "===================================================="
        ;;
    *)
        echo "❌ Invalid mode: $MODE"
        echo "Usage: $0 [normal|hotreload|dev]"
        echo ""
        echo "Modes:"
        echo "  normal    - Standard startup with live logs"
        echo "  hotreload - Auto-restart on file changes (requires air)"
        echo "  dev       - Multi-pane tmux/screen layout"
        exit 1
        ;;
esac

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

# Check if running from correct directory
if [ ! -f "docker-compose.yml" ]; then
    print_error "docker-compose.yml not found. Please run from project root directory."
    exit 1
fi

# Mode-specific checks
if [ "$MODE" = "hotreload" ]; then
    # Check if air is available
    if ! command -v air >/dev/null 2>&1 && [ ! -f "$HOME/go/bin/air" ]; then
        print_error "air not found. Please install it first:"
        echo "  go install github.com/air-verse/air@latest"
        echo "  # or add ~/go/bin to your PATH"
        exit 1
    fi
    # Use air from go/bin if not in PATH
    AIR_CMD="air"
    if ! command -v air >/dev/null 2>&1 && [ -f "$HOME/go/bin/air" ]; then
        AIR_CMD="$HOME/go/bin/air"
    fi
fi

# Step 1: Check for existing processes and clean up
print_status "Checking for existing processes..."

# Kill existing processes
pkill -f "uvicorn.*8000" 2>/dev/null || true
pkill -f "go run main.go" 2>/dev/null || true
if [ "$MODE" = "hotreload" ]; then
    pkill -f "air" 2>/dev/null || true
fi
print_success "Cleaned up existing processes"

# Step 2: Start RabbitMQ container using Rancher
print_status "Starting RabbitMQ container..."

# Check if Rancher is being used (based on user memory)
if command -v rancher >/dev/null 2>&1; then
    print_status "Using Rancher for container management..."
    rancher compose up -d rabbitmq
elif command -v docker >/dev/null 2>&1; then
    print_status "Using Docker Compose..."
    docker compose up -d rabbitmq
else
    print_error "Neither Rancher nor Docker found. Please install container management tools."
    exit 1
fi

# Wait for RabbitMQ to be ready
print_status "Waiting for RabbitMQ to start..."
for i in {1..30}; do
    if curl -s http://localhost:15672 >/dev/null 2>&1; then
        break
    fi
    if [ $i -eq 30 ]; then
        print_error "RabbitMQ failed to start within 30 seconds"
        exit 1
    fi
    sleep 1
done
print_success "RabbitMQ is ready (Web UI: http://localhost:15672)"

# Step 3: Activate Python virtual environment
print_status "Activating Python virtual environment..."
if [ ! -d ".venv" ]; then
    print_error "Virtual environment not found. Please run: python -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt"
    exit 1
fi

source .venv/bin/activate
print_success "Python virtual environment activated"

# Step 4: Start Go backend
if [ "$MODE" = "hotreload" ]; then
    print_status "Starting Go worker backend with hot reload..."
    # Air runs from project root and uses .air.toml config
    nohup $AIR_CMD > go-workers.log 2>&1 &
    GO_PID=$!
elif [ "$MODE" = "dev" ]; then
    print_status "Starting Go worker backend (dev mode - will use tmux)..."
    # For dev mode, we'll handle this in the tmux section
    GO_PID=""
else
    print_status "Starting Go worker backend..."
    cd workers
    if [ ! -f "go.mod" ]; then
        print_warning "Go modules not initialized. Running go mod tidy..."
        go mod tidy
    fi
    
    # Start Go server in background
    nohup go run main.go --port 8001 --webhook "http://localhost:8000/api/go-workers/webhook/events" > ../go-workers.log 2>&1 &
    GO_PID=$!
    cd ..
fi

# Handle dev mode with tmux
if [ "$MODE" = "dev" ]; then
    # Check for terminal multiplexer
    if command -v tmux >/dev/null 2>&1; then
        print_status "Using tmux for multi-pane terminal..."
        
        # Create new tmux session
        tmux new-session -d -s demo-event-bus
        
        # Split into 3 panes
        tmux split-window -h -t demo-event-bus
        tmux split-window -v -t demo-event-bus:0.1
        
        # Start Go backend in first pane
        tmux send-keys -t demo-event-bus:0.0 "cd workers && go run main.go --port 8001 --webhook 'http://localhost:8000/api/go-workers/webhook/events'" C-m
        
        # Wait for Go server to start
        print_status "Waiting for Go backend to start..."
        for i in {1..15}; do
            if curl -s http://localhost:8001/health >/dev/null 2>&1; then
                break
            fi
            if [ $i -eq 15 ]; then
                print_error "Go backend failed to start within 15 seconds"
                tmux kill-session -t demo-event-bus
                exit 1
            fi
            sleep 1
        done
        print_success "Go backend is ready"
        
        # Start Python web server in second pane
        tmux send-keys -t demo-event-bus:0.1 "source .venv/bin/activate && uvicorn app.web_server:app --reload --port 8000" C-m
        
        # Wait for web server to start
        print_status "Waiting for web server to start..."
        for i in {1..15}; do
            if curl -s http://localhost:8000/api/health >/dev/null 2>&1; then
                break
            fi
            if [ $i -eq 15 ]; then
                print_error "Web server failed to start within 15 seconds"
                tmux kill-session -t demo-event-bus
                exit 1
            fi
            sleep 1
        done
        print_success "Web server is ready"
        
        # Show status in third pane
        tmux send-keys -t demo-event-bus:0.2 "echo '🎯 DEMO EVENT BUS SYSTEM STATUS'" C-m
        tmux send-keys -t demo-event-bus:0.2 "echo '============================'" C-m
        tmux send-keys -t demo-event-bus:0.2 "echo '✅ RabbitMQ:     http://localhost:15672 (guest/guest)'" C-m
        tmux send-keys -t demo-event-bus:0.2 "echo '✅ Go Backend:   http://localhost:8001'" C-m
        tmux send-keys -t demo-event-bus:0.2 "echo '✅ Web Server:   http://localhost:8000'" C-m
        tmux send-keys -t demo-event-bus:0.2 "echo ''" C-m
        tmux send-keys -t demo-event-bus:0.2 "echo '🎮 READY! Open http://localhost:8000 to start'" C-m
        tmux send-keys -t demo-event-bus:0.2 "echo '🛑 Use: tmux kill-session -t demo-event-bus'" C-m
        
        # Set pane titles
        tmux select-pane -t demo-event-bus:0.0 -T "Go Backend"
        tmux select-pane -t demo-event-bus:0.1 -T "Web Server" 
        tmux select-pane -t demo-event-bus:0.2 -T "Status"
        
        # Attach to the session
        print_success "Starting tmux session with 3 panes..."
        echo ""
        echo "🎯 DEV MODE - TMUX SESSION:"
        echo "├─ Pane 0: Go Backend Logs"
        echo "├─ Pane 1: Web Server Logs"
        echo "└─ Pane 2: System Status"
        echo ""
        echo "Navigation: Ctrl+B then arrow keys to switch panes"
        echo "Detach: Ctrl+B then d"
        echo "Kill session: Ctrl+B then :kill-session"
        echo ""
        tmux attach-session -t demo-event-bus
        exit 0
    else
        print_warning "tmux not found. Install tmux for dev mode:"
        echo "  brew install tmux     # macOS"
        echo "  apt install tmux      # Ubuntu/Debian"
        echo ""
        print_status "Falling back to normal mode..."
        MODE="normal"
    fi
fi

# Wait for Go server to start (normal and hotreload modes)
if [ "$GO_PID" != "" ]; then
    print_status "Waiting for Go backend to start..."
    for i in {1..15}; do
        if curl -s http://localhost:8001/health >/dev/null 2>&1; then
            break
        fi
        if [ $i -eq 15 ]; then
            print_error "Go backend failed to start within 15 seconds"
            kill $GO_PID 2>/dev/null || true
            exit 1
        fi
        sleep 1
    done
    if [ "$MODE" = "hotreload" ]; then
        print_success "Go backend is ready with hot reload (http://localhost:8001)"
    else
        print_success "Go backend is ready (http://localhost:8001)"
    fi
fi

# Step 5: Start Python web server
print_status "Starting Python web server..."
nohup uvicorn app.web_server:app --reload --port 8000 > web-server.log 2>&1 &
WEB_PID=$!

# Wait for web server to start
print_status "Waiting for web server to start..."
for i in {1..15}; do
    if curl -s http://localhost:8000/api/health >/dev/null 2>&1; then
        break
    fi
    if [ $i -eq 15 ]; then
        print_error "Web server failed to start within 15 seconds"
        kill $WEB_PID 2>/dev/null || true
        kill $GO_PID 2>/dev/null || true
        exit 1
    fi
    sleep 1
done
print_success "Web server is ready (http://localhost:8000)"

# Step 6: Save process IDs for cleanup
echo $WEB_PID > .web_server.pid
echo $GO_PID > .go_backend.pid

# Step 7: Final status check
print_status "Performing final system check..."

# Check all services
RABBITMQ_STATUS=$(curl -s http://localhost:15672 >/dev/null 2>&1 && echo "✅" || echo "❌")
GO_STATUS=$(curl -s http://localhost:8001/health >/dev/null 2>&1 && echo "✅" || echo "❌")
WEB_STATUS=$(curl -s http://localhost:8000/api/health >/dev/null 2>&1 && echo "✅" || echo "❌")

echo ""
echo "🎯 SYSTEM STATUS"
echo "================"
echo "RabbitMQ:     $RABBITMQ_STATUS  http://localhost:15672 (guest/guest)"
echo "Go Backend:   $GO_STATUS  http://localhost:8001"
echo "Web Server:   $WEB_STATUS  http://localhost:8000"
echo ""
if [ "$MODE" = "hotreload" ]; then
    print_success "🔥 Demo Event Bus System with HOT RELOAD is fully operational!"
    echo ""
    echo "🎮 Open http://localhost:8000 to start using the application"
    echo "🔄 Files will auto-reload when you make changes:"
    echo "   • .go files → air rebuilds Go backend automatically"
    echo "   • .py files → uvicorn reloads web server automatically"
else
    print_success "Demo Event Bus System is fully operational!"
    echo ""
    echo "🎮 Open http://localhost:8000 to start using the application"
fi
echo ""
echo "📝 LIVE LOGS (Ctrl+C to stop all services)"
echo "=========================================="

# Create cleanup trap
cleanup() {
    echo ""
    print_warning "Stopping all services..."
    
    if [ "$GO_PID" != "" ]; then
        kill $WEB_PID $GO_PID 2>/dev/null || true
    else
        kill $WEB_PID 2>/dev/null || true
    fi
    
    # Stop air processes if in hotreload mode
    if [ "$MODE" = "hotreload" ]; then
        pkill -f "air" 2>/dev/null || true
    fi
    
    # Wait a moment for graceful shutdown
    sleep 2
    
    # Force kill if still running
    if [ "$GO_PID" != "" ]; then
        kill -9 $WEB_PID $GO_PID 2>/dev/null || true
    else
        kill -9 $WEB_PID 2>/dev/null || true
    fi
    
    print_success "All services stopped. Run ./stop_app.sh to clean up cache."
    exit 0
}

# Set trap for Ctrl+C
trap cleanup SIGINT SIGTERM

# Show aggregated logs from all services
print_status "Showing live logs from all services..."
echo "Use Ctrl+C to stop all services"
echo ""

# Use multitail if available, otherwise fallback to tail with prefixes
if command -v multitail >/dev/null 2>&1; then
    multitail -ci green -l "tail -f go-workers.log" -ci blue -l "tail -f web-server.log"
else
    # Fallback: tail both files with prefixes
    (tail -f go-workers.log | sed 's/^/[GO] /' &) 
    (tail -f web-server.log | sed 's/^/[WEB] /' &)
    
    # Keep the script running and wait for signals
    while true; do
        sleep 1
    done
fi