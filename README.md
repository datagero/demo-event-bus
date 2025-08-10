RabbitMQ learning app (topic exchange, routing keys, per-product queues, and "*.done" results)

Setup with uv

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

Run demos

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

Interactive demos

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

- If you see `ModuleNotFoundError: No module named 'app'`, run with `PYTHONPATH=.` as shown above.
- To change volume of burst messages: `COUNT=50 SLEEP=0`.
- To purge a demo queue from UI: Queues -> select queue -> Purge.

Queue Quest (gamified learning)

A tiny game built on RabbitMQ topics. The game master publishes quests; players accept and complete or fail them; a scoreboard tallies points in real time.

Terminals needed: open 3+ terminals.

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

What to watch in the UI
- Exchanges → `rte.topic` → bindings for `game.quest.*`
- Queues → `game.player.alice.q`, `game.player.bob.q`, `game.scoreboard.q`
- See Ready/Unacked on player queues while they work (`prefetch=1`), and result events flowing to the scoreboard queue

Tweak the game
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

Event-driven scenarios (narrative)

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

Clean start sequence
```bash
scripts/teardown.sh
export DOCKER_HOST=unix:///Users/$USER/.rd/docker.sock
docker compose up -d
scripts/wait_rabbit.sh localhost 5672 30
PYTHONPATH=. uvicorn app.web_server:app --reload --port 8000
```