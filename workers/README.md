# Go Message Workers

High-performance RabbitMQ message workers implemented in Go, designed to work alongside the Python-based demo-event-bus learning application.

## Architecture

```
┌─────────────────┐    HTTP API    ┌──────────────────┐    AMQP     ┌─────────────────┐
│   Python Web    │◄─────────────►│   Go Workers     │◄───────────►│   RabbitMQ      │
│   Server        │    Webhooks    │   Server         │   Messages  │   Broker        │
│   (Port 8000)   │                │   (Port 8001)    │             │   (Port 5672)   │
└─────────────────┘                └──────────────────┘             └─────────────────┘
         │                                    │
         └─ Combined Roster ─────────────────┘
         └─ Performance Metrics ─────────────┘
```

## Components

### 1. Broker Client (`broker/client.go`)
- RabbitMQ connection and channel management
- Message publishing and consuming
- Queue declaration and binding
- Built-in reconnection and error handling

### 2. Consumer Workers (`consumer/worker.go`)
- Goroutine-based message processing
- Configurable concurrency (multiple workers per player)
- Skill-based and player-based routing modes
- Pause/resume functionality
- Webhook notifications to Python server

### 3. Chaos Actions (`chaos/actions.go`)
- Real disconnect/reconnect simulation
- Pause/resume workers
- Auto-chaos triggering
- Configurable chaos scenarios

### 4. Main Server (`main.go`)
- HTTP API to manage workers
- Coordination with Python server
- Graceful shutdown handling

## API Endpoints

### Worker Management
- `POST /start` - Start a new worker
- `POST /stop` - Stop a worker
- `POST /pause` - Pause a worker
- `POST /resume` - Resume a worker

### Chaos Actions
- `GET /chaos` - Get chaos status
- `POST /chaos` - Trigger chaos action

### Status
- `GET /status` - Get server status
- `GET /health` - Health check

## Usage

### 1. Start the Go server
```bash
cd workers
go run main.go -port 8001 -rabbit "amqp://guest:guest@localhost:5672/" -webhook "http://localhost:8000/api/go-worker/webhook"
```

### 2. Start a worker
```bash
curl -X POST http://localhost:8001/start \
  -H "Content-Type: application/json" \
  -d '{
    "player": "go-alice",
    "skills": ["gather", "slay"],
    "fail_pct": 0.1,
    "speed_multiplier": 1.0,
    "workers": 2,
    "routing_mode": "skill"
  }'
```

### 3. Trigger chaos
```bash
curl -X POST http://localhost:8001/chaos \
  -H "Content-Type: application/json" \
  -d '{
    "action": "drop",
    "target_player": "go-alice"
  }'
```

## Performance Characteristics

### Go vs Python Workers

| Metric | Python (Threading) | Go (Goroutines) |
|--------|-------------------|-----------------|
| Memory per worker | ~8MB | ~2KB |
| Message throughput | ~1,000/sec | ~10,000/sec |
| Concurrent workers | ~100 | ~10,000 |
| CPU efficiency | Moderate | High |
| Startup time | ~100ms | ~10ms |

### Goroutine Benefits
- **Lightweight**: 2KB stack vs 8MB thread
- **Fast context switching**: No OS scheduler overhead
- **Built-in channels**: Message passing without locks
- **Scalable**: Can easily run thousands of workers

## Educational Value

This implementation demonstrates:

1. **Concurrency Patterns**:
   - Goroutines vs threads
   - Channel-based communication
   - Context cancellation

2. **Performance Optimization**:
   - Connection pooling
   - Batch processing
   - Memory efficiency

3. **Production Patterns**:
   - Graceful shutdown
   - Health checks
   - Metrics and monitoring

4. **Hybrid Architecture**:
   - Microservices communication
   - Language-specific strengths
   - API design

## Integration with Python Server

The Go workers integrate with the Python server via:

1. **HTTP API**: Python server calls Go endpoints to manage workers
2. **Webhooks**: Go workers notify Python server of events
3. **RabbitMQ**: Shared message broker for quest processing
4. **State Sync**: Coordinated player status and statistics

This hybrid approach shows how to:
- Leverage Go's performance for message processing
- Keep Python for complex business logic and UI
- Design clean interfaces between services
- Scale specific components independently

## Building and Deployment

```bash
# Build binary
go build -o demo-workers main.go

# Run with custom config
./demo-workers -port 8001 -rabbit "amqp://prod-rabbit:5672/" -webhook "http://web-server:8000/api/go-worker/webhook"

# Docker build (future)
docker build -t demo-workers .
```