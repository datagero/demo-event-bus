# Quest Timeline Status Documentation

This document defines the expected behavior for quest card timeline statuses in the Demo Event Bus system.

## Timeline Flow

Each quest should progress through the following timeline statuses:

### 1. **ISSUED** ⏸️
- **When**: Quest is first published to the exchange
- **Display**: "Issued by game-master"
- **Status**: `NEW` / `Ready` (yellow)
- **Source**: `game-master`
- **Event**: `QUEST_ISSUED`

### 2. **RECEIVED** 📨
- **When**: Worker accepts the message from RabbitMQ queue
- **Display**: "Received by {worker_name}"
- **Status**: `ACCEPTED` / `Processing` (blue) 
- **Source**: `go-worker:{worker_name}` or `worker:{worker_name}`
- **Event**: `message_accepted` broadcast event
- **RabbitMQ**: Message is acknowledged (ack)

### 3. **COMPLETED** ✅ or **FAILED** ❌
- **When**: Worker finishes processing (success or failure)
- **Display**: 
  - Success: "Completed by {worker_name} (+{points} pts)"
  - Failure: "Failed by {worker_name}"
- **Status**: `SUCCESS` / `Completed` (green) or `FAILED` / `Failed` (red)
- **Source**: `go-worker:{worker_name}` or `worker:{worker_name}`
- **Event**: `message_completed` or `message_failed` broadcast event
- **RabbitMQ**: Result published to completion exchange

## Implementation Requirements

### Go Workers (workers/consumer/worker.go)
1. **On Message Accept**: Call `notifyMessageEvent("accept", msg)` 
2. **On Completion**: Call `notifyMessageEvent("completed", resultMsg)` or `notifyMessageEvent("failed", resultMsg)`

### Python Webhook (app/go_workers.py)
1. **Receive "accept" events**: Broadcast as `message_accepted`
2. **Receive "completed" events**: Broadcast as `message_completed` 
3. **Receive "failed" events**: Broadcast as `message_failed`

### UI Timeline (app/web_static/index.html)
1. **message_accepted**: Add timeline entry "Received by {player}"
2. **message_completed**: Add timeline entry "Completed by {player} (+{points} pts)"
3. **message_failed**: Add timeline entry "Failed by {player}"

## Data Flow

```
[Quest Published] 
    ↓ (game-master)
[ISSUED: "Issued by game-master"]
    ↓ (RabbitMQ delivery)
[Go Worker Accepts] 
    ↓ (webhook: event="accept")
[RECEIVED: "Received by alice"]
    ↓ (processing work_sec)
[Go Worker Completes]
    ↓ (webhook: event="completed"/"failed") 
[COMPLETED: "Completed by alice (+5 pts)"]
```

## Webhook Event Format

Go workers send webhook events in this format:
```json
{
  "type": "message_event",
  "event": "accept|completed|failed", 
  "player": "alice",
  "message": {
    "case_id": "q-123456-789",
    "quest_type": "gather",
    "points": 5,
    ...
  }
}
```

## UI Event Handlers

The UI listens for these WebSocket broadcast events:
- `message_accepted` → Add "Received" timeline entry
- `message_completed` → Add "Completed" timeline entry  
- `message_failed` → Add "Failed" timeline entry

## Expected Behavior

**Normal Flow:**
1. Quest appears as "Ready" (yellow)
2. Worker accepts → "Received by alice" appears, status becomes "Processing" (blue)
3. Worker completes → "Completed by alice (+5 pts)" appears, status becomes "Completed" (green)

**Failure Flow:**  
1. Quest appears as "Ready" (yellow)
2. Worker accepts → "Received by alice" appears, status becomes "Processing" (blue)
3. Worker fails → "Failed by alice" appears, status becomes "Failed" (red)

## Common Issues

- **Missing "Received"**: Webhook not sending "accept" events or UI not handling `message_accepted`
- **Direct to Complete**: Worker completion events working but acceptance events missing
- **No Timeline Updates**: Webhook endpoint not receiving events or broadcast not working

## Testing Checklist

- [ ] Quest shows "Issued by game-master" 
- [ ] Quest shows "Received by {worker}" when accepted
- [ ] Quest shows "Completed by {worker} (+X pts)" when done
- [ ] Quest shows "Failed by {worker}" on failure
- [ ] Status colors change appropriately (yellow → blue → green/red)
- [ ] Timeline entries appear in correct chronological order