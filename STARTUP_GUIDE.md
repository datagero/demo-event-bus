# Demo Event Bus - Startup Guide

This guide explains how to start and stop the Demo Event Bus system using the provided scripts.

## Quick Start

### Start the System
```bash
./start_app.sh
```

### Stop the System
```bash
./stop_app.sh
```

## What the Scripts Do

### `start_app.sh`
1. **Clean up** - Stops any existing processes
2. **RabbitMQ** - Starts RabbitMQ container (using Rancher or Docker)
3. **Go Backend** - Starts the Go worker server on port 8001
4. **Web Server** - Starts the Python FastAPI server on port 8000
5. **Health Checks** - Verifies all services are running correctly

### `stop_app.sh`
1. **Stop Services** - Gracefully stops all running processes
2. **Stop Containers** - Stops RabbitMQ container
3. **Clean Cache** - Removes all cache and temporary files:
   - `.game_state_cache.pkl` (game state)
   - `go-workers.log` (Go backend logs)
   - `web-server.log` (web server logs)
   - Python `__pycache__` directories
4. **Verify Shutdown** - Confirms all services stopped

## System Requirements

- **Container Management**: Rancher (preferred) or Docker
- **Python**: Virtual environment in `.venv/`
- **Go**: Go runtime for the worker backend
- **Ports**: 5672, 15672 (RabbitMQ), 8000 (Web), 8001 (Go Backend)

## Service URLs

After starting:
- **Main Application**: http://localhost:8000
- **RabbitMQ Management**: http://localhost:15672 (guest/guest)
- **Go Backend API**: http://localhost:8001
- **Health Check**: http://localhost:8000/api/health

## Logs

During operation, logs are written to:
- `go-workers.log` - Go backend logs
- `web-server.log` - Python web server logs

View logs in real-time:
```bash
tail -f go-workers.log
tail -f web-server.log
```

## Troubleshooting

### Port Already in Use
If you get "port already in use" errors:
```bash
# Check what's using the ports
lsof -ti :8000 :8001 :5672 :15672

# Force kill if needed
sudo lsof -ti :8000 :8001 | xargs kill -9
```

### Container Issues
If RabbitMQ won't start:
```bash
# Check container status
docker ps -a
# or
rancher ps

# Force remove and restart
docker compose down --remove-orphans
./start_app.sh
```

### Virtual Environment Missing
If Python venv is missing:
```bash
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Manual Startup (Alternative)

If scripts don't work, start manually:

1. **RabbitMQ**:
   ```bash
   docker compose up -d rabbitmq
   ```

2. **Go Backend**:
   ```bash
   cd workers
   go run main.go --port 8001 --webhook "http://localhost:8000/api/go-workers/webhook/events"
   ```

3. **Web Server**:
   ```bash
   source .venv/bin/activate
   uvicorn app.web_server:app --reload --port 8000
   ```

## Features

The startup system includes:
- ✅ Automatic dependency checking
- ✅ Health verification for all services
- ✅ Graceful error handling
- ✅ Complete cache cleanup
- ✅ Process ID tracking
- ✅ Colored output for easy reading
- ✅ Support for both Rancher and Docker