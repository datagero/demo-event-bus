# 🔄 Development Workflow Guide

This guide explains the optimal development workflow for the Demo Event Bus project.

## 🎯 Quick Start for Developers

### 1. Initial Setup
```bash
# Start the main application (keep this terminal open)
./start_app.sh
```

### 2. Development Loop

When you modify code, use these commands in **separate terminals**:

```bash
# After modifying API server code
./restart_api.sh

# After modifying workers code  
./restart_workers.sh

# After modifying both
./restart_both.sh

# Check if everything is running
./check_status.sh
```

## 📊 Development Scripts

| Script | Purpose | When to Use |
|--------|---------|-------------|
| `./restart_api.sh` | Restart only API server | After changing files in `api-server/` |
| `./restart_workers.sh` | Restart only workers | After changing files in `workers/` |
| `./restart_both.sh` | Restart both services | After changes affecting both services |
| `./check_status.sh` | Check all service status | To verify services are running |

## 🎯 Best Practices

### ✅ DO
- **Keep `./start_app.sh` running** in your main terminal for continuous logs
- **Use restart scripts** in separate terminals for quick iteration  
- **Check logs** in the start_app terminal after restarts
- **Use `./check_status.sh`** when unsure about service status
- **Test functionality** at http://localhost:9000 after changes

### ❌ DON'T
- Stop and restart the entire application for small changes
- Kill processes manually - use the restart scripts
- Forget to check health endpoints after restarts
- Modify code while restart scripts are running

## 📝 Script Features

All development scripts include:
- **🔄 Smart process management**: Finds and stops only the target service
- **📊 Health checks**: Verifies services are responding after restart
- **🎯 Clear feedback**: Color-coded output showing progress
- **📋 Unified logging**: Logs continue flowing to existing log files
- **⚡ Fast rebuilds**: Only rebuilds the changed service
- **🔇 Noise filtering**: `start_app.sh` filters out repetitive API calls for cleaner logs

## 📋 Log Filtering

The main `start_app.sh` terminal shows **filtered** real-time logs:

**Filtered out (noise reduction):**
- `GET /api/rabbitmq/derived/metrics` - Called every 2 seconds
- `GET /api/rabbitmq/derived/scoreboard` - Called every 2 seconds  
- `GET /api/dlq/list` - Called every 3 seconds
- `📊 [RabbitMQ-Go] Derived metrics` - Internal metrics logging

**What you'll see:**
- 🔧 **[API SERVER]** - Worker lifecycle, message publishing, DLQ events, errors
- ⚡ **[WORKERS]** - All worker actions, message processing, failures

## 🐛 Troubleshooting

### Service Won't Start
```bash
# Check what's running
./check_status.sh

# Check logs
tail -f api-server.log     # API server logs
tail -f workers.log        # Workers logs
```

### Port Already in Use
```bash
# Find and kill process on port 9000 (API server)
lsof -ti:9000 | xargs kill -9

# Find and kill process on port 8001 (workers)  
lsof -ti:8001 | xargs kill -9

# Then restart
./restart_both.sh
```

### RabbitMQ Issues
```bash
# RabbitMQ is managed separately
docker-compose up -d       # Start RabbitMQ
docker-compose down        # Stop RabbitMQ
```

## 🏗️ Advanced Development

### With Hot Reloading (Air)
If you have [Air](https://github.com/cosmtrek/air) installed:

```bash
# Start with auto-reload
./start_app.sh --watch
```

This will automatically rebuild and restart services when files change.

### Manual Development
For manual control:

```bash
# Build API server manually
cd api-server
go build -o api-server-complete ./main.go

# Build workers manually  
cd workers
go build -o workers-complete ./main.go
```

## 📊 Monitoring During Development

### Real-time Service Health
- **Frontend**: http://localhost:9000/
- **API Health**: http://localhost:9000/health
- **Workers Health**: http://localhost:8001/health
- **RabbitMQ Management**: http://localhost:15672/

### Log Files
- **API Server**: `api-server.log`
- **Workers**: `workers.log`

### Quick Status Check
```bash
# One command to check everything
./check_status.sh
```

---

> 💡 **Pro Tip**: Set up terminal multiplexer (tmux/screen) or use multiple terminal tabs for the best experience - one for start_app.sh logs and others for development commands.