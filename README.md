RabbitMQ learning app (Queue Quest)

Queue Quest is an interactive learning application for RabbitMQ and event-driven systems. It includes an optional **Card Quest Challenge** - a gamified mode that teaches data engineering concepts through timed scenarios and scoring mechanics.

Setup

```bash
uv venv .venv
source .venv/bin/activate
uv pip install -r requirements.txt
```

Start RabbitMQ

```bash
docker compose up -d
```

RabbitMQ UI: [http://localhost:15672](http://localhost:15672) (guest/guest)

CLI demos (optional)

- Consumer (listens to `rte.retrieve.orders` on queue `demo.orders.q`):
  ```bash
  PYTHONPATH=. python scripts/rmq_consume_demo.py
  ```
- Publisher (sends a demo message to `rte.retrieve.orders`):
  ```bash
  PYTHONPATH=. python scripts/rmq_publish_demo.py
  ```
- Orchestrator wait (binds `demo.wait.q` to `rte.retrieve.*.done`, publishes a simulated `*.done`, and waits for a match):
  ```bash
  PYTHONPATH=. python scripts/rmq_wait_done_demo.py
  ```

Configuration (optional)

- `RTE_EXCHANGE`: exchange name. Default: `rte.topic`
- `RABBITMQ_URL`: AMQP URL. Default: `amqp://guest:guest@localhost:5672/%2F`

Example override:
```bash
export RTE_EXCHANGE=my.exchange
export RABBITMQ_URL=amqp://user:pass@localhost:5672/%2F
```

Interactive demos (optional)

- Slow processing (visualize Ready vs Unacked):
  ```bash
  PYTHONPATH=. python scripts/rmq_consume_slow.py
  # In a second terminal, send a burst to build backlog
  PYTHONPATH=. COUNT=15 SLEEP=0.05 python scripts/rmq_publish_burst.py
  ```
  Observe in the UI (Queues -> `demo.orders.slow.q`):
  - **Ready** grows as messages queue up
  - **Unacked** is ~1 due to `prefetch=1`
  - As the consumer acks, Ready drains and Unacked returns to 0

- Failures with Dead Letter Queue (DLQ):
  ```bash
  PYTHONPATH=. python scripts/rmq_consume_fail.py
  # In a second terminal
  PYTHONPATH=. python scripts/rmq_publish_demo.py
  ```
  Observe in the UI:
  - Queue `demo.orders.fail.q` briefly receives a message
  - Message is NACKed with `requeue=False`, routed to DLX `rte.dlx`
  - DLQ `demo.orders.dlq.q` receives the message and logs `[DLQ] got message:`
  - Use Queues -> `demo.orders.dlq.q` -> Get messages to inspect payloads

Tips

- If RabbitMQ UI says port busy, run `scripts/teardown.sh`.
- If Python can't import `app.*`, prefix commands with `PYTHONPATH=.`.
- Copy `.env.example` to `.env` and adjust if you need non-default RabbitMQ creds or API URL.
- New endpoints: `POST /api/chaos/arm`, `GET /api/chaos/state`, `GET /api/messages?status=pending|failed|dlq`, `POST /api/player/update`.
- To change volume of burst messages: `COUNT=50 SLEEP=0`.
- To purge a demo queue from UI: Queues -> select queue -> Purge.

Queue Quest (gamified learning)

A tiny game built on RabbitMQ topics. The game master publishes quests; players accept and complete or fail them; a scoreboard tallies points in real time.

Quickstart (web UI)

1) Ensure RabbitMQ is running (compose up)
2) Start the web server
   ```bash
   PYTHONPATH=. uvicorn app.web_server:app --reload --port 8000
   ```
3) Open http://localhost:8000
4) Use Quick Play → Quick Start to recruit Alice+Bob and start a wave, or recruit from Recruit pane

Keyboard shortcuts
- w: start wave with current parameters
- q: Quick Start (Alice+Bob + wave)
- 1/2/3: send one gather/slay/escort
- p/f/d: open Pending/Failed/DLQ tabs
- x: cycle/arm Chaos mode (drop → requeue → dlq → fail_early)

1) Scoreboard (Terminal A)
```bash
PYTHONPATH=. python scripts/game_scoreboard.py
```

2) Players (Terminal B/C)
- Alice: skilled at gather,slay; 20% fail rate
```bash
PYTHONPATH=. PLAYER=alice SKILLS=gather,slay FAIL_PCT=0.2 python scripts/game_player.py
```
- Bob: skilled at escort; 10% fail rate
```bash
PYTHONPATH=. PLAYER=bob SKILLS=escort FAIL_PCT=0.1 python scripts/game_player.py
```

3) Game master (Terminal D)
- Publish a wave of quests (mix of types, difficulties, points)
```bash
PYTHONPATH=. COUNT=20 DELAY=0.1 python scripts/game_master.py
```

What to watch in RabbitMQ UI
- Exchanges → `rte.topic` → bindings for `game.quest.*`
- Queues → `game.player.alice.q`, `game.player.bob.q`, `game.scoreboard.q`
- See Ready/Unacked on player queues while they work (`prefetch=1`), and result events flowing to the scoreboard queue

Tweak the game (CLI mode)
- Change skills: `SKILLS=slay` or `SKILLS=gather,escort`
- Change failure rate: `FAIL_PCT=0.5`
- Adjust quest volume/speed: `COUNT=100 DELAY=0`
- Edit difficulty timing in `scripts/game_master.py` (work_sec)

Internals
- Quests: `game.quest.<type>`
- Results: `game.quest.<type>.done` or `.fail`
- Per-player queues auto-delete when terminals close; scoreboard aggregates results in-memory

Web UI (FastAPI)

An interactive web UI to visualize events and control the game.

Install deps if not already done:
```bash
uv pip install -r requirements.txt
```

Run the web server:
```bash
PYTHONPATH=. uvicorn app.web_server:app --reload --port 8000
```
Open: http://localhost:8000

- Start players and master from the left panel
- Watch live events and scoreboard on the right
- Keep RabbitMQ running: `docker compose up -d`

How it works (game narrative)

Core loop
- Game master publishes quests to topic `game.quest.<type>` with difficulty/weight/points
- Players consume quests from shared per-skill queues (skill mode) or one queue per player (player mode)
- Players accept or skip; accepted quests are Unacked until ack; results publish to `game.quest.<type>.done|fail`
- Scoreboard subscribes to results and totals points

Key concepts
- Ready vs Unacked: Ready (queued); Unacked (delivered/processing until ack). On crash before ack → back to Ready
- Prefetch: limits parallel messages in-flight per consumer; set per-player to visualize contention
- Routing modes: Skill-based (single delivery fairness) vs Player-based (fanout duplicates)
- DLQ: NACK requeue=false sends to DLQ for investigation; requeue/purge from Messages panel

Scenarios (deterministic)
- Redelivery: simulate disconnect before ack → message returns to Ready → redelivered
- Requeue: NACK requeue=true → immediately back to Ready
- Duplicate / Both complete: use player-based routing to show each player gets/finishes their own copy
- DLQ poison: send a bad message to DLQ

Late-bind escort (backlog handoff)

This scenario demonstrates what happens when messages are published before a queue exists, and how a shared skill queue enables handoff between workers.

Steps
0) Reset state (clears UI metrics; players may keep running)
1) Publish escort quests while no escort queue exists → they are unroutable and dropped by the broker
2) Bob arrives and declares/binds `game.skill.escort.q` → starts consuming
3) Publish more escort quests → these are now queued and processed
4) Pause Bob → publish more → messages pile up as Ready in the escort queue
5) Dave arrives (also escort) → consumes from the same queue, draining the backlog and new messages

Why the early messages were lost
- In skill-based routing we bind a single shared queue per skill (e.g., `game.skill.escort.q`). If the queue does not exist yet and no binding for `game.quest.escort` is present, RabbitMQ drops the published messages by default (no alternate-exchange/mandatory flag).
- Once the first escort consumer declares/binds the queue, new messages are routed there and can be consumed by any worker for that skill. Pausing one worker simply builds a Ready backlog which another worker can drain later.

Run it
- From the UI: Scenarios → “Late-bind escort (backlog handoff)”
- Via API:
  ```bash
  curl -s -X POST localhost:8000/api/scenario/run \
    -H 'Content-Type: application/json' \
    -d '{"name":"late_bind_escort"}'
  ```

Optional: make “unroutable” visible
- If you want to see the early messages instead of dropping them, configure an alternate exchange (AE) on `rte.topic` and bind `unroutable.q` to that AE. Then you can inspect or replay from there.
  - In RabbitMQ UI: Exchanges → `rte.topic` → set `alternate-exchange` to `rte.ae`
  - Create `rte.ae` (type: fanout) and queue `unroutable.q` bound to it
  - Now anything published without a matching binding shows up in `unroutable.q`

Player downtime and redelivery (at-least-once)
- Story: A player (worker) crashes mid-quest. What happens to the message?
- What RabbitMQ does: With manual acks and prefetch=1, an in-flight message is Unacked while the player works. If the connection drops before ack, the broker requeues it. Another player (or the same one later) can pick it up. With a single player, the message remains Ready until it comes back.
- Try it:
  1) Start two players (e.g., alice and bob) in the web UI (or via terminal with `game_player.py`).
  2) Start a master wave. Watch Events in the web UI and Queues in RabbitMQ.
  3) As soon as you see “ACCEPT: alice -> …”, terminate Alice (Ctrl+C) or close her terminal.
  4) Observe: `Unacked` drops, `Ready` increases on `game.player.alice.q`, then Bob accepts and continues. If Alice was alone, the message stays Ready until she returns.
- Concepts: at-least-once delivery; manual ack; consumer prefetch limits parallel in-flight work.

Private channels, least-privilege and targeted commands
- Need: Players should report status centrally without seeing others’ data; the central system should target one player or a team without broadcasting secrets.
- Building blocks:
  - Vhosts and per-user credentials: use a dedicated vhost, create a user per player with fine-grained permissions (configure in RabbitMQ UI: Admin → Users/Permissions). Use TLS (`amqps://`).
  - Topic partitioning: publish public quests on `game.quest.*`. Players only have read perms on these. Players only have write perms on `game.quest.*.(done|fail)` and `status.<playerId>`.
  - Direct, private commands: create a direct exchange `game.cmd`. Each player binds a private queue `cmd.<playerId>` with routing key `<playerId>`. The central system publishes to `game.cmd` with `routing_key=<playerId>` to reach only that player. For teams, use topic keys like `team.<teamId>.<playerId>` or `cmd.team.<teamId>`.
  - Status reporting: players publish sanitized metrics/events to `status.<playerId>` on `game.status` (topic). Central binds `status.*`; players have no read perms on `status.*`.
  - Sensitive fields: encrypt at the application layer for fields that must remain confidential from other principals; or use per-recipient keys.
- Try it (UI-only, no code changes required):
  1) Create exchange `game.cmd` (type: direct).
  2) Create queue `cmd.alice`, bind to `game.cmd` with routing key `alice`.
  3) Use the UI to Publish to exchange `game.cmd` with `routing_key=alice` and any JSON body. Only `cmd.alice` receives it. (To fully handle commands, extend `game_player.py` to also consume `cmd.<playerId>`.)

Dead-letter queues (DLQs) in practice
- Why DLQs exist:
  - Poison messages: data your code can’t process → NACK (requeue=false) and capture for investigation.
  - TTL expiration: messages that waited too long → expire to DLQ for triage.
  - Max retries: after N processing attempts, stop retrying and DLQ the message to avoid hot-looping.
  - Unroutable/audit: alternate exchange to collect misrouted payloads.
- Demo 1: Poison message → DLQ (already included)
  - Run: `PYTHONPATH=. python scripts/rmq_consume_fail.py` then publish (e.g., `scripts/rmq_publish_demo.py`).
  - Observe: processing queue NACKs with `requeue=False`; message lands in DLQ (`demo.orders.dlq.q`), visible in UI and logs.
- Demo 2: TTL → DLQ (no code changes)
  - In RabbitMQ UI: Policies → Add policy `ttl-demo` (apply to queues, pattern `^ttl.demo.q$`, definitions: `message-ttl=5000`, `dead-letter-exchange=rte.dlx`, `dead-letter-routing-key=ttl.dlq`).
  - Create queue `ttl.demo.q`, bind to `rte.topic` with a key you publish to (e.g., `rte.retrieve.orders`). Also create/bind DLQ `ttl.dlq.q` to `rte.dlx` with key `ttl.dlq`.
  - Publish some messages and don’t consume them. After ~5s they expire and appear in `ttl.dlq.q`.
- Demo 3: Retry with backoff, then DLQ (UI-only)
  - Create `work.q` (DLX=`retry.ex`, DLK=`rk`), `retry.q` (TTL=5000, DLX back to `rte.topic`/`work.q` binding), and a consumer that intentionally fails first N attempts using a counter (or use `rmq_consume_fail.py`).
  - Observe `x-death` headers count retries; once threshold reached, route to a terminal DLQ for manual handling.

Takeaways
- Downtime isn’t data loss: unacked messages are requeued and delivered later (at-least-once). Idempotency is crucial.
- Privacy and targeting are modeled via exchanges, bindings, and broker permissions: central can address a single player or cohort without leaking to others.
- DLQs are for safe failure handling and triage: poison data, timeouts, and exhausted retries go somewhere observable instead of looping or vanishing.

Rancher Desktop / Docker socket

If you use Rancher Desktop, set the Docker CLI to its socket so compose works:
```bash
export DOCKER_HOST=unix:///Users/$USER/.rd/docker.sock
```

## Card Quest Challenge (Optional Plugin)

The **Card Quest Challenge** is a plug-and-play game mode that adds time-pressured scenarios to teach advanced data engineering concepts. It's completely optional and can be enabled or disabled without affecting the core functionality.

### How it Works

- **Game Timer**: Every 30 seconds, draw a random card with immediate effects
- **Card Types**: 
  - 🟢 **Green** (Opportunities): New Hire, Cross-Training, Upgrade Queue, Scale Up, Alternate Route
  - 🟡 **Yellow** (Load & Growth): Message Surge, Skill Boom, Multi-Skill Wave, Steady Growth  
  - 🔴 **Red** (Problems): Character Quits, Queue Blocked, Retention Shrink, Network Partition, Exchange Misbind
  - ⚫ **Black** (Chaos Events): Storm of Skills, Mass Resignation, Broker Glitch, Retention Purge, Random Replay
- **Scoring**: Start with 1000 points, lose points for unroutable messages (-5), DLQ messages (-3), expired messages (-10)
- **Win Condition**: Survive the full duration (default 5 minutes) with maximum points

### Enable/Disable Card Game

**To Enable** (default):
```bash
# Card game is enabled by default when app/card_game.py exists
# The Card Quest Challenge panel will appear in the web UI
```

**To Disable**:
```bash
# Rename the card game module to disable it completely
mv app/card_game.py app/card_game_enabled.py
mv app/card_game_disabled.py app/card_game.py
# Restart the web server - the Card Quest Challenge panel will be hidden
```

**To Re-enable**:
```bash
# Restore the card game module
mv app/card_game.py app/card_game_disabled.py  
mv app/card_game_enabled.py app/card_game.py
# Restart the web server
```

### Educational Goals

- **Green Cards**: Teach scaling, redundancy, and proactive configuration
- **Yellow Cards**: Teach load management and planning for growth  
- **Red Cards**: Teach fault tolerance and rapid incident handling
- **Black Cards**: Teach resilience under extreme chaos scenarios

The card system transforms static learning into an engaging, time-pressured game where players must react to real-world data engineering scenarios while maintaining 100% message delivery.

## Clean start sequence
```bash
scripts/teardown.sh
export DOCKER_HOST=unix:///Users/$USER/.rd/docker.sock
docker compose up -d
scripts/wait_rabbit.sh localhost 5672 30
PYTHONPATH=. uvicorn app.web_server:app --reload --port 8000
```