# Dead Letter Queue (DLQ) Strategy & Retry Mechanism Documentation

## Overview

The Event Bus demo implements a comprehensive Dead Letter Queue (DLQ) system with intelligent retry mechanisms to handle different types of message failures. This document explains the strategy, implementation, and usage.

## DLQ Categories

### 1. 📍 **Unroutable Messages**
- **Cause**: Messages published when no queue exists to receive them
- **Queue**: `game.dlq.unroutable.q`
- **Strategy**: Direct reissue to original queue
- **Reasoning**: Routing issue is likely fixed (queue recreated)

### 2. ❌ **Failed Messages**  
- **Cause**: Messages rejected by workers due to processing failures
- **Queue**: `game.dlq.failed.q`
- **Strategy**: Retry queue with TTL delay
- **Reasoning**: Immediate reissue may fail again; delay allows for recovery

### 3. ⏰ **Expired Messages**
- **Cause**: Messages that exceeded TTL (time-to-live) 
- **Queue**: `game.dlq.expired.q`
- **Strategy**: Direct reissue to original queue
- **Reasoning**: TTL timeout likely due to temporary worker unavailability

### 4. 🔄 **Retrying Messages**
- **Cause**: Messages in active retry cycle
- **Queue**: `game.dlq.retry.q`
- **Strategy**: Automatic retry via TTL expiration
- **Reasoning**: Built-in retry mechanism with backoff

## Reissue Strategies

### Strategy Matrix

| DLQ Type | Reissue Method | Target | Timeline Message | Color |
|----------|----------------|--------|------------------|-------|
| Unroutable | Direct | `game.quest.{type}` | `"↻ Sent to REISSUED DLQ"` | 🟡 Yellow |
| Failed | Retry Queue | `dlq.retry` | `"🔄 Sent to RETRY DLQ"` | 🟡 Yellow |
| Expired | Direct | `game.quest.{type}` | `"↻ Sent to REISSUED DLQ"` | 🟡 Yellow |

### Implementation Details

#### Direct Reissue (Unroutable/Expired)
```
Message Flow: DLQ → Original Queue → Worker
Routing Key: game.quest.{quest_type}
Timeline: Immediate processing
```

#### Retry Queue (Failed)  
```
Message Flow: DLQ → Retry Queue → (TTL) → Original Queue → Worker
Routing Key: dlq.retry → (auto) → game.quest.{quest_type}
Timeline: Delayed retry with backoff
```

## Retry Queue Mechanism

### Configuration
- **Queue**: `game.dlq.retry.q`
- **TTL**: Configurable delay (default: based on setup)
- **Dead Letter Exchange**: `game.skill`
- **Dead Letter Routing Key**: `quest.retry`

### How It Works
1. Failed message is sent to retry queue via `dlq.retry` routing key
2. Message sits in retry queue for TTL duration
3. TTL expiration triggers dead letter routing
4. Message is automatically sent back to `game.skill` exchange
5. Message is routed to appropriate worker queue for retry

### Retry Tracking
- **retry_count**: Incremented with each retry attempt
- **reissued_at**: Timestamp of reissue operation
- **reissued_from_dlq**: Flag indicating DLQ origin

## User Interface

### Quest Log Entries
- `"📍 {quest-id} (gather) → UNROUTABLE"` - Message went to unroutable DLQ
- `"❌ {quest-id} (gather) → FAILED"` - Message went to failed DLQ  
- `"🔄 {quest-id} sent to RETRY queue"` - Failed message reissued
- `"↻ {quest-id} reissued to original queue"` - Direct reissue

### Quest Card Timeline
- `"{time}: 📍 Sent to UNROUTABLE DLQ"` (red)
- `"{time}: ❌ Sent to FAILED DLQ"` (red)
- `"{time}: 🔄 Sent to RETRY DLQ"` (yellow)
- `"{time}: ↻ Sent to REISSUED DLQ"` (yellow)

### Refresh Buttons
- **DLQ Controls**: Bulk reissue operations
- **Quest Board Cards**: Individual message reissue with smart routing

## Implementation Files

### Backend
- `api-server/internal/api/handlers/dlq.go` - DLQ management and reissue logic
- `workers/consumer/worker.go` - Message processing and failure handling
- `workers/broker/client.go` - Queue setup with DLX configuration

### Frontend  
- `frontend/static/index.html` - UI components and real-time updates
- Quest Board cards with refresh buttons
- DLQ statistics and management controls
- Real-time logging and timeline updates

## Educational Benefits

### Message Lifecycle Visibility
1. **Publishing**: Message creation and routing
2. **Processing**: Worker acceptance and handling  
3. **Failure**: DLQ categorization and logging
4. **Recovery**: Reissue strategies and retry cycles

### Debugging Support
- Complete message tracing from publish to completion/failure
- Visual indicators for different failure types
- Real-time status updates and logging
- Comprehensive error categorization

## Best Practices

### When to Use Each Strategy
- **Direct Reissue**: Infrastructure issues (routing, availability)
- **Retry Queue**: Application failures (processing errors, validation)
- **Manual Intervention**: Persistent failures requiring investigation

### Monitoring
- Track retry counts to identify problematic messages
- Monitor DLQ accumulation for system health
- Use timeline data for failure pattern analysis

### Configuration
- Adjust retry TTL based on failure characteristics
- Set max retry limits to prevent infinite loops
- Configure appropriate dead letter routing for recovery

## Conclusion

The DLQ strategy provides:
- **Intelligent Recovery**: Different strategies for different failure types
- **Complete Visibility**: Real-time monitoring and logging
- **Educational Value**: Clear demonstration of message handling patterns
- **Production Readiness**: Robust error handling and retry mechanisms

This system demonstrates enterprise-grade message queue patterns while maintaining educational clarity and ease of use.