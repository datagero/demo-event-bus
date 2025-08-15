#!/bin/bash

# Queue Quest Debug Log Monitor
# Captures console logs from both frontend and backend for development

LOG_DIR="./logs"
mkdir -p "$LOG_DIR"

echo "🔍 Queue Quest Debug Log Monitor"
echo "================================"
echo "📁 Logs will be saved to: $LOG_DIR"
echo ""

# Function to add timestamps to log lines
add_timestamp() {
    while IFS= read -r line; do
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $line"
    done
}

# Function to monitor API server logs
monitor_api_logs() {
    echo "🚀 Monitoring API server logs..."
    tail -f api-server.log 2>/dev/null | add_timestamp > "$LOG_DIR/api-server-debug.log" &
    API_LOG_PID=$!
}

# Function to monitor worker logs
monitor_worker_logs() {
    echo "⚡ Monitoring worker logs..."
    tail -f workers.log 2>/dev/null | add_timestamp > "$LOG_DIR/workers-debug.log" &
    WORKER_LOG_PID=$!
}

# Function to create a combined debug log
create_combined_log() {
    echo "📋 Creating combined debug log..."
    (
        echo "=== QUEUE QUEST DEBUG SESSION STARTED: $(date) ==="
        echo "=== API Server Logs ==="
        tail -50 api-server.log 2>/dev/null | add_timestamp
        echo ""
        echo "=== Worker Logs ==="
        tail -50 workers.log 2>/dev/null | add_timestamp
        echo ""
        echo "=== Real-time monitoring started ==="
    ) > "$LOG_DIR/combined-debug.log"
    
    # Continue monitoring in real-time
    (
        tail -f api-server.log 2>/dev/null | sed 's/^/[API] /' | add_timestamp
    ) >> "$LOG_DIR/combined-debug.log" &
    COMBINED_API_PID=$!
    
    (
        tail -f workers.log 2>/dev/null | sed 's/^/[WORKER] /' | add_timestamp  
    ) >> "$LOG_DIR/combined-debug.log" &
    COMBINED_WORKER_PID=$!
}

# Function to filter interesting logs
create_filtered_log() {
    echo "🎯 Creating filtered debug log (webhooks, quest acceptance, errors)..."
    (
        echo "=== FILTERED DEBUG LOG: $(date) ==="
        echo "=== Showing: Webhooks, Quest Acceptance, Errors, WebSocket Messages ==="
        echo ""
    ) > "$LOG_DIR/filtered-debug.log"
    
    # Monitor and filter logs in real-time
    (
        tail -f api-server.log workers.log 2>/dev/null | grep -E "(🔔|webhook|player_accept|quest_issued|error|ERROR|WebSocket|🎯|🔍)" | add_timestamp
    ) >> "$LOG_DIR/filtered-debug.log" &
    FILTERED_PID=$!
}

# Function to show current status
show_status() {
    echo ""
    echo "📊 Debug Log Status:"
    echo "   📁 Log directory: $LOG_DIR"
    echo "   🚀 API server log: $(wc -l < "$LOG_DIR/api-server-debug.log" 2>/dev/null || echo 0) lines"
    echo "   ⚡ Worker log: $(wc -l < "$LOG_DIR/workers-debug.log" 2>/dev/null || echo 0) lines"  
    echo "   📋 Combined log: $(wc -l < "$LOG_DIR/combined-debug.log" 2>/dev/null || echo 0) lines"
    echo "   🎯 Filtered log: $(wc -l < "$LOG_DIR/filtered-debug.log" 2>/dev/null || echo 0) lines"
    echo ""
    echo "🌐 Debug Console: file://$(pwd)/debug-console.html"
    echo ""
}

# Function to cleanup on exit
cleanup() {
    echo ""
    echo "🛑 Stopping debug log monitoring..."
    kill $API_LOG_PID $WORKER_LOG_PID $COMBINED_API_PID $COMBINED_WORKER_PID $FILTERED_PID 2>/dev/null
    echo "✅ Debug logs saved in $LOG_DIR"
    show_status
    exit 0
}

# Set up signal handlers
trap cleanup SIGINT SIGTERM

# Start monitoring
monitor_api_logs
monitor_worker_logs  
create_combined_log
create_filtered_log

echo "✅ Debug logging started!"
echo ""
echo "📋 Available logs:"
echo "   • $LOG_DIR/api-server-debug.log - API server only"
echo "   • $LOG_DIR/workers-debug.log - Workers only" 
echo "   • $LOG_DIR/combined-debug.log - Everything combined"
echo "   • $LOG_DIR/filtered-debug.log - Webhooks, quest acceptance, errors"
echo ""
echo "🌐 Open debug console: file://$(pwd)/debug-console.html"
echo ""
echo "Press Ctrl+C to stop monitoring and save logs."

# Show real-time filtered output
echo "🎯 Real-time filtered log (webhooks, quest acceptance):"
echo "======================================================="
tail -f "$LOG_DIR/filtered-debug.log"