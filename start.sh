#!/bin/bash

# Demo Event Bus - Simple Launcher
# Shows available startup modes

echo "🚀 Demo Event Bus - Startup Options"
echo "===================================="
echo ""
echo "Choose your startup mode:"
echo ""
echo "1️⃣  Normal Mode (default)"
echo "   ./start_app.sh"
echo "   • Live logs in terminal"
echo "   • Ctrl+C to stop everything"
echo ""
echo "2️⃣  Hot Reload Mode ⭐"
echo "   ./start_app.sh hotreload"
echo "   • Auto-restart on .go/.py file changes"
echo "   • Requires 'air' tool (go install github.com/air-verse/air@latest)"
echo ""
echo "3️⃣  Development Mode"
echo "   ./start_app.sh dev"
echo "   • Multi-pane tmux layout"
echo "   • Separate logs for each service"
echo ""
echo "🛑 To stop everything:"
echo "   ./stop_app.sh"
echo ""
echo "💡 Quick start: ./start_app.sh hotreload"