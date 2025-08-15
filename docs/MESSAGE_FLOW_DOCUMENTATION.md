# Queue Quest Message Flow & .done Queue Documentation

## Overview

This document explains the complete message flow in Queue Quest, with special focus on the `.done` queue concept and why we handle completion messages the way we do.

## Message Flow Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Frontend/API  │    │   RabbitMQ      │    │   Go Workers   │    │   API Server    │
│                 │    │   Exchanges     │    │                 │    │   (Webhooks)    │
└─────────────────┘    └─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │                       │
         │ 1. Publish Quest      │                       │                       │
         │ ───────────────────► │                       │                       │
         │   game.quest.gather   │                       │                       │
         │                       │                       │                       │
         │                       │ 2. Route to Worker    │                       │
         │                       │ ───────────────────► │                       │
         │                       │   game.skill.gather.q │                       │
         │                       │                       │                       │
         │                       │                       │ 3. Send Accept Webhook│
         │                       │                       │ ───────────────────► │
         │                       │                       │   (player_accept)     │
         │                       │                       │                       │
         │                       │ 4. Publish Result     │                       │
         │                       │ ◄─────────────────── │                       │
         │                       │   game.quest.gather.done                     │
         │                       │                       │                       │
         │                       │ 5. Route Back to      │                       │
         │                       │    Worker Queue       │                       │
         │                       │ ───────────────────► │                       │
         │                       │   game.skill.gather.q │                       │
         │                       │                       │                       │
         │                       │                       │ 6. Send Complete Hook │
         │                       │                       │ ───────────────────► │
         │                       │                       │   (result_done)       │
         │                       │                       │                       │
         │ ◄─────────────────────────────────────────────────────────────────── │
         │ 7. WebSocket: player_accept, result_done                              │
```

## The `.done` Queue Problem & Solution

### What Happens Without `.done` Bindings

**Problem**: When a worker completes a quest, it publishes a completion message with routing key `game.quest.gather.done`. If there's no queue bound to receive this message, it becomes **unroutable** and goes to the DLQ.

```
Worker completes quest → Publishes "game.quest.gather.done" → No queue listens → UNROUTABLE DLQ
```

### Why We Need `.done` Bindings

**Solution**: We bind each worker's skill queue to **both** incoming quests AND completion messages:

```go
// In workers/broker/client.go line 126-138
// Bind to incoming quest messages (e.g., game.quest.gather)
err = c.channel.QueueBind(queueName, routingKey, ExchangeName, false, nil)

// Also bind to completion messages (e.g., game.quest.gather.done)
completionRoutingKey := routingKey + ".done"  
err = c.channel.QueueBind(queueName, completionRoutingKey, ExchangeName, false, nil)
```

This means:
- `game.skill.gather.q` receives both:
  - `game.quest.gather` (new quests)
  - `game.quest.gather.done` (completion notifications)

### Why We Ignore Completion Messages

**In workers/consumer/worker.go lines 230-234:**

```go
// IGNORE completion messages - these should not be processed as new quests
if msg.EventStage == "QUEST_COMPLETED" || msg.EventStage == "QUEST_FAILED" {
    log.Printf("📄 [Go Worker] %s ignoring completion message %s (EventStage: %s)", 
               w.Config.PlayerName, msg.CaseID, msg.EventStage)
    return true // ACK the completion message but don't process it
}
```

**Why this approach?**

1. **Prevents Infinite Loops**: Without this check, Alice would:
   - Complete a gather quest
   - Publish `game.quest.gather.done`
   - Receive her own completion message
   - Try to "process" it as a new quest
   - Fail it (because it's not a real quest)
   - Send it to DLQ
   - Infinite loop!

2. **Prevents Unroutable Messages**: The `.done` binding ensures completion messages have somewhere to go instead of becoming unroutable.

## Message Types & Event Stages

### Initial Quest Messages
```json
{
  "case_id": "single-gather-1234567890",
  "event_stage": "",
  "quest_type": "gather", 
  "difficulty": 1,
  "work_sec": 2,
  "points": 5,
  "source": "frontend_send_one"
}
```
**Routing Key**: `game.quest.gather`

### Completion Messages (Success)
```json
{
  "case_id": "single-gather-1234567890", 
  "event_stage": "QUEST_COMPLETED",
  "status": "SUCCESS",
  "quest_type": "gather",
  "points": 5,
  "player": "alice",
  "source": "go-worker:alice"
}
```
**Routing Key**: `game.quest.gather.done`

### Completion Messages (Failure)
```json
{
  "case_id": "single-gather-1234567890",
  "event_stage": "QUEST_FAILED", 
  "status": "FAILED",
  "quest_type": "gather",
  "points": 0,
  "player": "alice",
  "source": "go-worker:alice"
}
```
**Routing Key**: `game.quest.gather.fail`

## Alternative Approaches Considered

### Option 1: Separate Results Exchange ❌
**Problem**: Would require workers to consume from multiple exchanges, increasing complexity.

### Option 2: Dedicated Results Queue ❌ 
**Problem**: All completion messages would go to one queue, losing the skill-based routing benefits.

### Option 3: No Completion Messages ❌
**Problem**: No way to track quest completion for metrics and UI updates.

### Option 4: Only Webhook Notifications ❌
**Problem**: Tight coupling between workers and API server; no audit trail in message broker.

## Current Approach Benefits ✅

1. **No Message Loss**: Completion messages have a destination
2. **Educational Value**: Shows real-world RabbitMQ routing patterns  
3. **Audit Trail**: All messages flow through RabbitMQ for inspection
4. **Loose Coupling**: Workers publish results; API server consumes via webhooks
5. **Scalability**: Each skill has its own routing path

## Your Question: Why Not Ignore All Messages to .done Queues?

You suggested ignoring all messages that go to `.done` queues. This would work, but:

**Current approach is better because:**

1. **More Explicit**: We filter by `event_stage` which clearly identifies the message type
2. **Future-Proof**: We might want `.done` queues to handle other message types later
3. **Debugging**: We can see exactly what types of messages are being ignored
4. **Flexibility**: Different event stages could be handled differently in the future

**Your approach would work too:**
```go
// Alternative: Ignore all messages to .done queues
if strings.Contains(delivery.RoutingKey, ".done") {
    log.Printf("📄 [Go Worker] %s ignoring .done message", w.Config.PlayerName)
    return true
}
```

But the current approach is more maintainable and explicit about intent.

## Summary

The `.done` queue concept exists to:
1. **Prevent unroutable completion messages** 
2. **Provide an audit trail** of all quest completions
3. **Enable educational demonstration** of RabbitMQ routing patterns
4. **Support future extensibility** for completion message handling

The worker ignores these completion messages to prevent infinite loops while ensuring they have a proper destination in RabbitMQ.