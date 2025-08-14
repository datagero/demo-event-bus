# 🚀 Demo Event Bus - Go-First RabbitMQ Educational Platform

A comprehensive educational platform demonstrating **RabbitMQ message broker patterns** with **direct broker integration** and **zero abstraction layers**. Built with Go for performance and educational transparency.

## 🎯 Educational Philosophy

This platform is designed for **RabbitMQ education** with these core principles:

- **🔍 Direct RabbitMQ Integration**: Data comes directly from RabbitMQ Management API
- **🚫 Zero Abstraction Layers**: Students see real broker internals, not app-level caches
- **📊 Educational Transparency**: All responses clearly indicate their RabbitMQ source
- **⚡ Native Operations**: Chaos engineering uses direct RabbitMQ commands
- **🎓 Learning Focus**: Minimize the gap between students and RabbitMQ

## 🏗️ Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Frontend      │    │   Go API Server  │    │   RabbitMQ      │
│   (React-like)  │◄──►│   Port 9000      │◄──►│   + Mgmt API    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                       ┌──────────────────┐
                       │   Go Workers     │
                       │   Port 8001      │
                       └──────────────────┘
```

### 🔧 Components

- **Go API Server** (Port 9000): Native RabbitMQ integration with Management API
- **Go Workers** (Port 8001): Message processors with skill-based routing
- **RabbitMQ** (Port 5672): Message broker with Management UI (Port 15672)
- **Frontend**: Real-time WebSocket UI for educational visualization

## 🚀 Quick Start

### Prerequisites

- **Go 1.21+**
- **Docker & Docker Compose** (for RabbitMQ)
- **curl** (for testing)

### 1. Start the Complete System

```bash
# Clone and enter directory
git clone <repository>
cd demo-event-bus

# Start everything (RabbitMQ + Go API + Go Workers)
# This will show live logs from both services with colored prefixes
./start_app.sh
```

### 2. Access the Platform

- **🌐 Frontend**: http://localhost:9000/
- **🔧 API Documentation**: http://localhost:9000/api/
- **🐰 RabbitMQ Management**: http://localhost:15672/ (guest/guest)

### 3. Stop the System

```bash
./stop_app.sh
```

### 🔄 Development Quick Reference

When modifying code during development:
```bash
# Restart just the API server after changes
./restart_api.sh

# Restart just the workers after changes  
./restart_workers.sh

# Restart both services
./restart_both.sh

# Check status of all services
./check_status.sh
```
> **💡 Tip**: Keep `./start_app.sh` running in your main terminal and use restart scripts in separate terminals for the best development experience.

## 🎮 Quick Educational Examples

### Create Workers (Alice & Bob)
```bash
curl -X POST http://localhost:9000/api/players/quickstart \
  -H 'Content-Type: application/json' \
  -d '{"preset":"alice_bob"}'
```

### Publish a Quest Message
```bash
curl -X POST http://localhost:9000/api/publish \
  -H 'Content-Type: application/json' \
  -d '{
    "routing_key": "game.quest.gather",
    "payload": {
      "case_id": "quest-1",
      "quest_type": "gather", 
      "points": 5
    }
  }'
```

### View RabbitMQ-Direct Metrics
```bash
curl http://localhost:9000/api/rabbitmq/metrics | jq
```

### Run Educational Scenarios
```bash
# Late-bind pattern: publish quest before worker exists
curl -X POST http://localhost:9000/api/scenario/run \
  -H 'Content-Type: application/json' \
  -d '{"scenario":"late-bind-escort"}'

# Quest wave publishing
curl -X POST http://localhost:9000/api/scenario/run \
  -H 'Content-Type: application/json' \
  -d '{"scenario":"quest-wave","params":{"wave_size":5}}'
```

### Chaos Engineering (RabbitMQ-Native)
```bash
# Purge a queue directly via Management API
curl -X POST http://localhost:9000/api/chaos/arm \
  -H 'Content-Type: application/json' \
  -d '{
    "action": "rmq_purge_queue",
    "target_queue": "game.skill.gather.q"
  }'

# Block connections
curl -X POST http://localhost:9000/api/chaos/arm \
  -H 'Content-Type: application/json' \
  -d '{"action": "rmq_block_connection"}'
```

## 📊 Educational Features

### 🎯 RabbitMQ-Direct Data Sources

All application data comes directly from RabbitMQ:

- **📈 Metrics**: Live queue statistics from Management API
- **👥 Consumers**: Real consumer tracking without app caches  
- **📨 Messages**: Direct queue inspection and message peeking
- **⚡ Chaos**: Native RabbitMQ operations (not simulations)

### 🔍 Message Flow Patterns

**Skill-Based Routing**:
```
Message: game.quest.gather → game.skill.gather.q → alice (gather skill)
Message: game.quest.slay   → game.skill.slay.q   → bob (slay skill)
```

**Late-Binding Pattern**:
```
1. Publish game.quest.escort (no consumers)
2. Message queues in game.skill.escort.q
3. Create escort worker
4. Worker immediately processes queued message
```

### 💀 Dead Letter Queue (DLQ) System

Native RabbitMQ DLQ features:
- **x-dead-letter-exchange**: Route failed messages
- **x-message-ttl**: Message expiration
- **x-death** header inspection: Retry counts and failure reasons
- **Retry queues**: Backoff patterns with chained TTLs

### ⚡ Chaos Engineering

Educational chaos actions using **real RabbitMQ commands**:
- `rmq_delete_queue`: Delete queues via Management API
- `rmq_purge_queue`: Clear messages from queues  
- `rmq_block_connection`: Close AMQP connections
- `rmq_unbind_queue`: Remove queue bindings

## 🔧 API Reference

### Core Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | System health check |
| `/api/rabbitmq/metrics` | GET | **RabbitMQ-direct** queue metrics |
| `/api/players/quickstart` | POST | Create worker presets |
| `/api/publish` | POST | **Native** message publishing |
| `/api/chaos/arm` | POST | **RabbitMQ-native** chaos actions |
| `/api/scenario/run` | POST | Educational scenarios |

### RabbitMQ Integration Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/rabbitmq/queues` | GET | Live queue data from Management API |
| `/api/rabbitmq/consumers` | GET | Real consumer tracking |
| `/api/rabbitmq/exchanges` | GET | Exchange configuration |
| `/api/rabbitmq/messages/:queue` | GET | Direct message peeking |

### DLQ Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/dlq/setup` | POST | Configure native DLQ topology |
| `/api/dlq/list` | GET | List DLQ messages by category |
| `/api/dlq/reissue` | POST | Replay messages from DLQ |

## 🎓 Educational Scenarios

### 1. Basic Message Flow
```bash
# Create workers → Publish messages → Watch processing
./start_go_complete.sh
curl -X POST localhost:9000/api/players/quickstart -d '{"preset":"alice_bob"}'
curl -X POST localhost:9000/api/publish -d '{"routing_key":"game.quest.gather","payload":{"points":5}}'
```

### 2. Late-Binding Pattern
```bash
# Publish first, create worker later
curl -X POST localhost:9000/api/scenario/run -d '{"scenario":"late-bind-escort"}'
```

### 3. Chaos Engineering
```bash
# Publish messages, then disrupt
curl -X POST localhost:9000/api/scenario/run -d '{"scenario":"chaos-test"}'
```

### 4. Load Testing
```bash
# High-volume message publishing
curl -X POST localhost:9000/api/scenario/run -d '{"scenario":"load-test","params":{"message_count":100}}'
```

## 🔍 Monitoring & Debugging

### RabbitMQ Management UI
Access http://localhost:15672/ (guest/guest) to see:
- **Queues**: Real-time message counts
- **Exchanges**: Routing configuration  
- **Connections**: Active AMQP connections
- **Channels**: Per-connection channels

### Application Logs
```bash
# View Go API Server logs
docker logs <api-server-container>

# View Go Workers logs  
docker logs <workers-container>

# View RabbitMQ logs
docker logs demo-event-bus-rabbitmq-1
```

### Direct RabbitMQ Queries
```bash
# Check queue status
curl -u guest:guest http://localhost:15672/api/queues

# Peek messages
curl -u guest:guest http://localhost:15672/api/queues/%2F/game.skill.gather.q/get \
  -X POST -d '{"count":5,"ackmode":"ack_requeue_false"}'

# View consumers
curl -u guest:guest http://localhost:15672/api/consumers
```

## 🏗️ Development

### Building Components

```bash
# Build Go API Server
cd api-server
go build -o api-server-complete ./main.go

# Build Go Workers  
cd workers
go build -o workers-complete ./main.go
```

### Hot Reloading (Development)

```bash
# API Server with hot reload (requires 'air')
cd api-server
air

# Workers with hot reload
cd workers  
go run main.go --port 8001 --webhook "http://localhost:9000/api/go-workers/webhook/events"
```

### Development Workflow

For the best development experience, keep the main application running in one terminal and use these scripts for individual service restarts:

#### 🎯 Recommended Workflow

1. **Start the main application** (keep this running):
   ```bash
   ./start_app.sh
   ```

2. **When you modify API server code**, restart just the API server:
   ```bash
   ./restart_api.sh
   ```

3. **When you modify workers code**, restart just the workers:
   ```bash
   ./restart_workers.sh
   ```

4. **To restart both services at once**:
   ```bash
   ./restart_both.sh
   ```

#### 📝 Development Script Features

- **🔄 Quick restarts**: Only rebuild and restart the changed service
- **📊 Health checks**: Automatic verification that services are responding
- **📋 Unified logging**: All logs continue to flow to api-server.log and workers.log
- **🎯 Clear feedback**: Color-coded output showing restart progress
- **⚡ Fast iteration**: No need to restart RabbitMQ or other services

#### 💡 Development Tips

- **Keep start_app.sh running** in your main terminal - it shows **filtered live logs from both API server and workers**
- **Use restart scripts** in separate terminals for quick iteration
- **Watch the combined logs** in start_app.sh terminal with colored prefixes:
  - 🔧 [API SERVER] - API server logs with timestamps (periodic API calls filtered)
  - ⚡ [WORKERS] - Workers service logs with timestamps (all worker actions shown)
- **Check health** at http://localhost:9000/health after restarts

### Environment Variables

```bash
# API Server
export RABBITMQ_URL="amqp://guest:guest@localhost:5672/"
export RABBITMQ_API_URL="http://localhost:15672/api"
export WORKERS_URL="http://localhost:8001"

# Workers
export RABBITMQ_URL="amqp://guest:guest@localhost:5672/"
export WEBHOOK_URL="http://localhost:9000/api/go-workers/webhook/events"
```

## 📚 Learning Resources

### RabbitMQ Concepts Demonstrated

1. **Exchanges & Routing**: Topic exchanges with skill-based routing keys
2. **Queues**: Durable queues with skill-specific binding patterns
3. **Consumers**: Multiple consumers per queue with prefetch limits
4. **Dead Letter Queues**: Native DLQ with retry policies
5. **Management API**: Direct broker introspection and control
6. **Connection Management**: AMQP connection lifecycle
7. **Message Properties**: Headers, TTL, routing keys, persistence

### Message Patterns Shown

- **Work Queues**: Task distribution among workers
- **Publish/Subscribe**: Topic-based message distribution  
- **RPC Pattern**: Request/response via queues
- **Dead Letter Handling**: Failed message recovery
- **Delayed Messages**: TTL-based message timing

## 🗂️ Project Structure

```
demo-event-bus/
├── api-server/           # Go API Server (main backend)
│   ├── internal/
│   │   ├── api/handlers/ # HTTP endpoint handlers
│   │   ├── clients/      # RabbitMQ & Workers clients
│   │   ├── websocket/    # WebSocket hub
│   │   └── models/       # Data models
│   └── main.go          # Server entry point
├── workers/             # Go Workers (message processors)
│   ├── consumer/        # Worker implementation
│   ├── chaos/          # Chaos engineering
│   └── main.go         # Workers entry point
├── legacy/             # Archived Python code
│   └── python/         # Original Python implementation
├── start_app.sh         # Main startup script
├── stop_app.sh         # Cleanup script
└── docker-compose.yml  # RabbitMQ service
```

## 🔧 Troubleshooting

### Common Issues

**Port conflicts**:
```bash
# Check what's using ports
lsof -i :9000 -i :8001 -i :5672 -i :15672

# Stop conflicting services
./stop_app.sh
```

**RabbitMQ connection issues**:
```bash
# Restart RabbitMQ
docker-compose down && docker-compose up -d

# Check RabbitMQ logs
docker logs demo-event-bus-rabbitmq-1
```

**Go build issues**:
```bash
# Update dependencies
cd api-server && go mod tidy
cd workers && go mod tidy
```

### Health Checks

```bash
# Test all services
curl http://localhost:9000/health      # Go API Server
curl http://localhost:8001/health      # Go Workers  
curl http://localhost:15672/api/overview # RabbitMQ
```

## 📊 Testing

The project includes a comprehensive testing framework with organized test suites for reliable validation of complex distributed systems scenarios.

### Quick Testing

```bash
# Fast unit tests (recommended during development)
./tests/run_unit_tests.sh

# All tests with service checking
./tests/run_all_tests.sh

# Specific test categories
./tests/run_all_tests.sh unit           # Unit tests only
./tests/run_all_tests.sh integration    # Integration tests
./tests/run_all_tests.sh api-server     # All API server tests
```

### Test Framework Features

- **Organized Structure**: Tests categorized by type (unit/integration/scenarios)
- **Test Isolation**: Independent tests with automatic cleanup
- **Service Dependencies**: Automatic service health checking
- **Common Utilities**: Shared helpers for consistent testing patterns
- **Multiple Execution Options**: From fast feedback to comprehensive validation

For comprehensive testing documentation, see [Testing Framework Guide](tests/README.md).

## 🤝 Contributing

This is an educational project. Contributions should maintain the **RabbitMQ-direct** philosophy:

1. **Minimize abstractions** - Data should come from RabbitMQ where possible
2. **Educational transparency** - Clearly show RabbitMQ operations
3. **Direct integration** - Use Management API for real broker data
4. **Clear attribution** - Mark data sources in API responses
5. **Test Coverage** - New features should include appropriate tests using the shared framework

## 📄 License

Educational use. See LICENSE file for details.

## 🙏 Acknowledgments

Built for RabbitMQ education with focus on **direct broker integration** and **educational transparency**. Designed to minimize the gap between students and RabbitMQ internals.

---

**🎯 Educational Goal**: Students learn RabbitMQ by seeing real broker operations, not application abstractions.

**🚀 Quick Start**: `./start_app.sh` → Visit http://localhost:9000/`