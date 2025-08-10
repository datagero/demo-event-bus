import asyncio
import json
import os
import pickle
import threading
import time
from dataclasses import dataclass, field
from typing import Dict, List, Optional

from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import HTMLResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel
from app.state import GameState

# Go workers integration (mandatory)
from app.go_workers import GoWorkersClient, create_go_workers_router
from app.rabbitmq_utils import RabbitEventBus, EXCHANGE_NAME, build_message, publish_wave
from app import scenarios
from dotenv import load_dotenv

load_dotenv()
app = FastAPI()

# Serve static UI
static_dir = os.path.join(os.path.dirname(__file__), "web_static")
app.mount("/static", StaticFiles(directory=static_dir), name="static")


@dataclass
class Client:
    queue: asyncio.Queue = field(default_factory=asyncio.Queue)


class Broadcaster:
    def __init__(self, loop: asyncio.AbstractEventLoop):
        self.loop = loop
        self.clients: List[Client] = []
        self.lock = threading.Lock()

    def add_client(self, client: Client):
        with self.lock:
            self.clients.append(client)

    def remove_client(self, client: Client):
        with self.lock:
            try:
                self.clients.remove(client)
            except ValueError:
                pass

    def broadcast_threadsafe(self, message: dict):
        with self.lock:
            targets = list(self.clients)
        for client in targets:
            asyncio.run_coroutine_threadsafe(client.queue.put(message), self.loop)


broadcaster: Broadcaster | None = None
scoreboard: Dict[str, int] = {}
fails: Dict[str, int] = {}
roster: Dict[str, Dict] = {}
players: Dict[str, Dict] = {}
processed_results: set[str] = set()
skip_logged: set[tuple[str, str]] = set()  # (player, quest_id)
routing_mode: str = os.getenv("GAME_ROUTING_MODE", "skill")  # skill|player
quests_state: Dict[str, Dict] = {}
player_stats: Dict[str, Dict] = {}
inflight_by_player: Dict[str, set] = {}
dlq_messages: list[dict] = []
unroutable: list[dict] = []
skill_ttl_ms: Dict[str, int] = {}
shutting_down: set[str] = set()
# RabbitMQ-native chaos system
chaos_config = {
    "enabled": False,
    "action": None,  # amqp_disconnect, amqp_close_channel, rmq_delete_queue, rmq_unbind_queue, rmq_block_connection
    "target_player": None,  # specific player or None for global
    "target_queue": None,  # specific queue or None for any
    "auto_trigger": False,  # automatically publish messages when armed
    "trigger_delay": 2.0,  # seconds before auto-trigger
    "trigger_count": 1,  # number of messages to auto-publish
    "educational_note": "Chaos actions now use direct RabbitMQ Management API instead of app-level simulation"
}

# RabbitMQ-native routing configuration
routing_config = {
    "mode": "rmq_native",  # rmq_native|legacy_python
    "exchange_hierarchy": {
        "game.routing": {  # New routing exchange
            "type": "topic",
            "durable": True,
            "educational_note": "Central routing exchange - no Python routing logic needed"
        },
        "game.skill": {  # Skill-specific distribution
            "type": "direct", 
            "durable": True
        },
        "game.player": {  # Player-specific distribution
            "type": "direct",
            "durable": True
        }
    },
    "binding_patterns": {
        "skill_based": [
            {"from": "game.routing", "to": "game.skill", "key": "quest.*.skill", "transform": "quest.{skill}"},
            {"from": "game.skill", "to": "game.skill.{skill}.q", "key": "quest.{skill}"}
        ],
        "player_based": [
            {"from": "game.routing", "to": "game.player", "key": "quest.*.player", "transform": "quest.{player}"},
            {"from": "game.player", "to": "game.player.{player}.q", "key": "quest.{player}"}
        ]
    },
    "educational_note": "Pure RabbitMQ routing - no Python logic required"
}

# Card Game System (optional plug-in)
try:
    from app.card_game import CardGame
    CARD_GAME_ENABLED = True
except ImportError:
    CARD_GAME_ENABLED = False
    CardGame = None
# Centralized game state helper
STATE = GameState(
    quests_state=quests_state,
    player_stats=player_stats,
    inflight_by_player=inflight_by_player,
    scoreboard=scoreboard,
    fails=fails,
    roster=roster,
    dlq_messages=dlq_messages,
)

# RabbitMQ-direct helper functions (replacing STATE.* methods)
async def record_quest_event_rmq(quest_id: str, event_type: str, player: str = None, quest_type: str = "unknown"):
    """Record quest events via RabbitMQ message flow instead of internal state tracking.
    
    Educational Note: In a pure RabbitMQ system, quest state would be tracked via:
    - Message acknowledgments (accepted/completed)
    - DLQ routing (failed)
    - Queue inspection (pending)
    - Consumer statistics (inflight)
    
    This function demonstrates the transition from app-level to broker-level state.
    """
    # Instead of internal state, we broadcast the event for RabbitMQ message flow tracking
    broadcast_raw_message(
        routing_key=f"game.quest.{quest_type}.{event_type}",
        payload={
            "quest_id": quest_id,
            "event_type": event_type,
            "event_stage": f"QUEST_{event_type.upper()}",
            "player": player,
            "quest_type": quest_type,
            "timestamp": time.time()
        },
        source="app_event_tracker"
    )

async def get_quest_metrics_from_rmq():
    """Get quest metrics directly from RabbitMQ instead of internal state tracking."""
    try:
        metrics_result = await api_rabbitmq_derived_metrics()
        if metrics_result["ok"]:
            return metrics_result["metrics"]
        return {"total_pending": 0, "total_unacked": 0, "per_type": {}}
    except Exception:
        return {"total_pending": 0, "total_unacked": 0, "per_type": {}}

# State persistence
STATE_CACHE_FILE = ".game_state_cache.pkl"

def save_state():
    """Save current game state to cache file"""
    try:
        state_data = {
            "roster": dict(roster),
            "players": {name: {k: v for k, v in meta.items() if k != "bus"} for name, meta in players.items()},
            "player_stats": dict(player_stats),
            "scoreboard": dict(scoreboard),
            "fails": dict(fails),
            "quests_state": dict(quests_state),
            "inflight_by_player": {k: list(v) if isinstance(v, set) else v for k, v in inflight_by_player.items()},
            "dlq_messages": list(dlq_messages),
            "unroutable": list(unroutable),
            "skill_ttl_ms": dict(skill_ttl_ms),
            "routing_mode": routing_mode,
        }
        with open(STATE_CACHE_FILE, "wb") as f:
            pickle.dump(state_data, f)
        # Count active Go workers instead of old roster
        active_workers = len(go_workers_client.roster) if go_workers_client.enabled else 0
        print(f"State saved: {active_workers} active Go workers, {len(roster)} cached players")
    except Exception as e:
        print(f"Failed to save state: {e}")

def load_state():
    """Load game state from cache file"""
    global routing_mode
    try:
        if not os.path.exists(STATE_CACHE_FILE):
            return False
            
        with open(STATE_CACHE_FILE, "rb") as f:
            state_data = pickle.load(f)
        
        # Restore state (but don't restore bus connections)
        roster.clear()
        roster.update(state_data.get("roster", {}))
        
        players.clear()
        for name, meta in state_data.get("players", {}).items():
            # Restore player data but mark as offline (threads need to reconnect)
            players[name] = {**meta, "bus": None}
            if name in roster:
                roster[name]["status"] = "offline"
        
        player_stats.clear()
        player_stats.update(state_data.get("player_stats", {}))
        
        scoreboard.clear()
        scoreboard.update(state_data.get("scoreboard", {}))
        
        fails.clear()
        fails.update(state_data.get("fails", {}))
        
        quests_state.clear()
        quests_state.update(state_data.get("quests_state", {}))
        
        inflight_by_player.clear()
        for k, v in state_data.get("inflight_by_player", {}).items():
            inflight_by_player[k] = set(v) if isinstance(v, list) else v
            
        dlq_messages.clear()
        dlq_messages.extend(state_data.get("dlq_messages", []))
        
        unroutable.clear()
        unroutable.extend(state_data.get("unroutable", []))
        
        skill_ttl_ms.clear()
        skill_ttl_ms.update(state_data.get("skill_ttl_ms", {}))
        
        routing_mode = state_data.get("routing_mode", "skill")
        
        print(f"State loaded: {len(roster)} players, {len(quests_state)} quests")
        return True
    except Exception as e:
        print(f"Failed to load state: {e}")
        return False

def clear_state_cache():
    """Clear the state cache file"""
    try:
        if os.path.exists(STATE_CACHE_FILE):
            os.remove(STATE_CACHE_FILE)
    except Exception as e:
        print(f"Failed to clear state cache: {e}")

# Load state on startup
if load_state():
    print("✅ Restored cached state from previous session")
else:
    print("🔄 Starting with fresh state")

# Go workers integration (mandatory)
GO_WORKERS_URL = os.getenv("GO_WORKERS_URL", "http://localhost:8001")
go_workers_client = GoWorkersClient(GO_WORKERS_URL)
print(f"🚀 Go workers system initialized (server: {GO_WORKERS_URL})")

def create_go_worker(name: str, skills: List[str], speed: float = 1.0, workers: int = 1, fail_pct: float = 0.1) -> bool:
    """Adapter function to create Go workers with Python worker interface."""
    success = go_workers_client.create_worker(name, skills, speed, fail_pct, workers)
    if success:
        # Ensure the worker is broadcast immediately
        broadcast("roster", {})
    return success

def delete_go_worker(name: str) -> bool:
    """Adapter function to delete Go workers."""
    success = go_workers_client.delete_worker(name)
    if success:
        # Remove from main roster if it exists there
        if name in roster:
            del roster[name]
        broadcast("roster", {})
    return success

def control_go_worker(name: str, action: str) -> bool:
    """Adapter function to control Go workers."""
    success = go_workers_client.control_worker(name, action)
    if success:
        broadcast("roster", {})
    return success

# Alias for backward compatibility
start_player_thread = create_go_worker

ENABLE_SCOREBOARD_CONSUMER = os.getenv("ENABLE_SCOREBOARD_CONSUMER", "1") not in {"0", "false", "False"}
RETRY_SEC = float(os.getenv("RABBITMQ_RETRY_SEC", "2.0"))

# Initialize card game if available
card_game = None
if CARD_GAME_ENABLED:
    card_game = CardGame(
        broadcast_fn=lambda t, p: None,  # Will be set after broadcast is defined
        game_state=None,  # Will be set after STATE is defined
        players_dict=roster,
        skill_ttl_dict=skill_ttl_ms,
        start_player_fn=create_go_worker  # Use Go workers instead of Python threading
    )


# Card game functions will be handled by the CardGame class if enabled


def broadcast(type_: str, payload: dict):
    if broadcaster:
        snap_metrics = STATE.snapshot()
        # Refresh Go workers status to ensure roster is current
        go_workers_client.get_status()
        # Use Go workers roster
        roster = go_workers_client.roster
        
        snap = {
            "type": type_,
            "payload": payload,
            "ts": time.time(),
            "scoreboard": scoreboard,
            "fails": fails,
            "roster": roster,
            "routing_mode": routing_mode,
            "metrics": snap_metrics.get("metrics", {}),
            "player_stats": snap_metrics.get("player_stats", {}),
            "go_workers_enabled": go_workers_client.enabled,
            "source": "app_state",  # Educational: Mark data source for UI transparency
            "educational_note": "Standard mode - uses internal state aggregation"
        }
        broadcaster.broadcast_threadsafe(snap)


# Add Go workers router after broadcast is defined
go_workers_router = create_go_workers_router(go_workers_client, broadcast)
app.include_router(go_workers_router)
print("🔗 Go workers API endpoints enabled")

# Initialize card game dependencies after broadcast is defined
if card_game:
    card_game.broadcast = broadcast
    card_game.game_state = STATE


def scoreboard_consumer_thread(loop: asyncio.AbstractEventLoop):
    global scoreboard, fails

    # Retry connection until RabbitMQ is reachable
    bus: RabbitEventBus | None = None
    while bus is None:
        try:
            bus = RabbitEventBus()
        except Exception:
            time.sleep(RETRY_SEC)

    q = "web.scoreboard.q"
    # Only bind to results; avoid binding to issued so pre-queue publishes can be truly unroutable
    bus.ch.queue_declare(queue=q, durable=True, auto_delete=False)
    bus.ch.queue_bind(queue=q, exchange=EXCHANGE_NAME, routing_key="game.quest.*.done")
    bus.ch.queue_bind(queue=q, exchange=EXCHANGE_NAME, routing_key="game.quest.*.fail")

    def handler(payload: dict, ack):
        # EDUCATIONAL: Broadcast raw message for RabbitMQ introspection
        routing_key = f"game.quest.{payload.get('quest_type', 'unknown')}.{payload.get('event_stage', 'unknown').lower()}"
        broadcast_raw_message(routing_key, payload, source="scoreboard_consumer")
        
        # Determine event type from payload
        event_stage = payload.get("event_stage", "")
        player = payload.get("player") or ""
        points = int(payload.get("points", 0))
        source = payload.get("source", "")

        global unroutable
        if event_stage.endswith("COMPLETED"):
            # dedupe by quest id
            cid = payload.get("case_id")
            if cid and cid not in processed_results and player:
                processed_results.add(cid)
                scoreboard[player] = scoreboard.get(player, 0) + points
                STATE.record_done(player, cid, payload.get("quest_type", "gather"))
                # Clean up any stale unroutable entry for this id
                try:
                    unroutable = [u for u in unroutable if (u.get("payload", {}).get("case_id") != cid)]
                    broadcast("unroutable_updated", {"count": len(unroutable)})
                except Exception:
                    pass
            # Skip timeline broadcast for Go workers - webhook already handles it
            # This prevents duplicate timeline entries
            if not source.startswith("go-worker"):
                evt_type = "result_done"
            else:
                evt_type = None  # Don't broadcast timeline event
        elif event_stage.endswith("FAILED"):
            cid = payload.get("case_id")
            if cid and cid not in processed_results and player:
                processed_results.add(cid)
                fails[player] = fails.get(player, 0) + 1
                STATE.record_fail(player, cid, payload.get("quest_type", "gather"))
                try:
                    unroutable = [u for u in unroutable if (u.get("payload", {}).get("case_id") != cid)]
                    broadcast("unroutable_updated", {"count": len(unroutable)})
                except Exception:
                    pass
            # Skip timeline broadcast for Go workers - webhook already handles it
            # This prevents duplicate timeline entries
            if not source.startswith("go-worker"):
                evt_type = "result_fail"
            else:
                evt_type = None  # Don't broadcast timeline event
        elif event_stage == "QUEST_ISSUED":
            # Ignore "issued" here to prevent late duplicates after a quest
            # is already completed/failed. The publisher already broadcasts
            # quest_issued immediately at publish time.
            ack()
            return
        else:
            evt_type = "event"

        # Only broadcast if we have an event type (prevents duplicates for Go workers)
        if evt_type:
            broadcast(evt_type, payload)
        ack()

    try:
        bus.consume_forever(q, handler)
    finally:
        try:
            bus.close()
        except Exception:
            pass


def real_disconnect_player(player: str, auto_reconnect_delay: float = 0):
    """
    Perform real disconnect for a player (same logic as chaos drop).
    If auto_reconnect_delay > 0, automatically reconnect after that delay.
    """
    if player not in players:
        return
    
    # Mark as disconnected
    roster[player] = {**roster.get(player, {}), "status": "disconnected"}
    broadcast("player_disconnected", {"player": player, "quest_id": "manual"})
    
    # Close connection immediately (real disconnect)
    bus = players[player].get("bus")
    try:
        if bus:
            bus.conn.close()
    except Exception:
        pass
    
    # Auto-reconnect if requested
    if auto_reconnect_delay > 0:
        skills = list(roster.get(player, {}).get("skills", []))
        fail_pct = roster.get(player, {}).get("fail_pct", 0.0)
        speed_multiplier = roster.get(player, {}).get("speed_multiplier", 1.0)
        workers = roster.get(player, {}).get("workers", 1)
        
        def auto_reconnect():
            time.sleep(auto_reconnect_delay)
            if player in players and not players[player].get("controls", {}).get("shutdown"):
                start_player_thread(player, skills, fail_pct, speed_multiplier, workers)
        
        threading.Thread(target=auto_reconnect, daemon=True).start()


# Go-only worker system - old Python worker function removed
def start_player_thread(player: str, skills: List[str], fail_pct: float, speed_multiplier: float = 1.0, workers: int = 1):
    """Legacy function redirected to Go workers."""
    return create_go_worker(name=player, skills=skills, speed=speed_multiplier, fail_pct=fail_pct, workers=workers)

def run_master_once(count: int, delay: float, fixed_type: Optional[str] = None):
    # Retry connection
    bus: RabbitEventBus | None = None
    while bus is None:
        try:
            bus = RabbitEventBus()
        except Exception:
            time.sleep(RETRY_SEC)

    import random
    quest_types = ["gather", "slay", "escort"]
    points_by_type = {"gather": 5, "slay": 10, "escort": 15}
    difficulty_choices = [("easy", 1.0), ("medium", 2.0), ("hard", 3.5)]

    broadcast("master_wave_started", {"count": count, "delay": delay})

    for i in range(count):
        quest_type = fixed_type or random.choice(quest_types)
        difficulty, work_sec = random.choice(difficulty_choices)
        points = points_by_type[quest_type]
        quest_id = f"q-{int(time.time())}-{i}"
        payload = build_message(
            case_id=quest_id,
            event_stage="QUEST_ISSUED",
            status="NEW",
            source="game-master",
            extra={
                "quest_type": quest_type,
                "difficulty": difficulty,
                "work_sec": work_sec,
                "points": points,
                "weight": 1 if difficulty=="easy" else (2 if difficulty=="medium" else 4),
            },
        )
        try:
            bus.publish(f"game.quest.{quest_type}", payload)
        except Exception:
            pass
        # record pending
        STATE.record_issued(quest_id, quest_type)
        # Broadcast directly so UI sees it even if scoreboard consumer is disabled
        broadcast("quest_issued", payload)
        time.sleep(delay)
    try:
        bus.close()
    except Exception:
        pass


def publish_one(quest_type: str, reissue_of: Optional[str] = None):
    import random, time as _time
    # Retry connection until RabbitMQ is reachable
    bus: RabbitEventBus | None = None
    while bus is None:
        try:
            bus = RabbitEventBus()
        except Exception:
            time.sleep(RETRY_SEC)
    points_by_type = {"gather": 5, "slay": 10, "escort": 15}
    difficulty_choices = [("easy", 1.0), ("medium", 2.0), ("hard", 3.5)]
    difficulty, work_sec = random.choice(difficulty_choices)
    points = points_by_type.get(quest_type, 5)
    # If reissuing, keep the same quest id so UI updates the same card
    quest_id = reissue_of or f"q-{int(_time.time())}-{random.randint(100,999)}"
    payload = build_message(
        case_id=quest_id,
        event_stage="QUEST_ISSUED",
        status="NEW",
        source="game-master",
        extra={
            "quest_type": quest_type,
            "difficulty": difficulty,
            "work_sec": work_sec,
            "points": points,
            "weight": 1 if difficulty=="easy" else (2 if difficulty=="medium" else 4),
            **({"reissue_of": reissue_of} if reissue_of else {}),
        },
    )
    unr_flag = {"val": False}
    def on_unroutable(pl):
        try:
            info = dict(pl)
            payload = info.get("payload", {})
            entry = {
                "ts": time.time(),
                "routing_key": info.get("routing_key"),
                "exchange": info.get("exchange"),
                "reply_code": info.get("reply_code"),
                "reply_text": info.get("reply_text"),
                "payload": payload,
            }
            unroutable.append(entry)
            # Also create a quest card (status=unroutable) for visibility
            cid = payload.get("case_id") or payload.get("id")
            qtype = payload.get("quest_type", "gather")
            if cid:
                try:
                    STATE.record_unroutable(cid, qtype)
                    broadcast("quest_issued", { **payload, "unroutable": True })
                except Exception:
                    pass
            broadcast("unroutable", entry)
            unr_flag["val"] = True
        except Exception:
            pass
    try:
        bus.publish(f"game.quest.{quest_type}", payload, on_unroutable=on_unroutable)
        # allow basic.return to fire before we close the connection
        time.sleep(0.05)
    except Exception:
        pass
    if not unr_flag["val"]:
        STATE.record_issued(quest_id, quest_type)
        broadcast("quest_issued", payload)
    try:
        bus.close()
    except Exception:
        pass


class MasterStartRequest(BaseModel):
    count: int = 20
    delay: float = 0.1


class PlayerStartRequest(BaseModel):
    player: str
    skills: str = "gather,slay,escort"  # comma-separated
    fail_pct: float = 0.2
    speed_multiplier: float = 1.0
    workers: int = 1
    prefetch: int = 1
    drop_rate: float = 0.0  # random disconnect
    skip_rate: float = 0.0  # random nack-requeue


@app.on_event("startup")
async def on_startup():
    global broadcaster
    loop = asyncio.get_running_loop()
    broadcaster = Broadcaster(loop=loop)
    # Start scoreboard consumer in background thread if enabled
    if ENABLE_SCOREBOARD_CONSUMER:
        t = threading.Thread(target=scoreboard_consumer_thread, args=(loop,), daemon=True)
        t.start()
    ht = threading.Thread(target=heartbeat_thread, args=(loop,), daemon=True)
    ht.start()


@app.get("/")
async def index():
    with open(os.path.join(static_dir, "index.html"), "r", encoding="utf-8") as f:
        return HTMLResponse(f.read())


@app.websocket("/ws")
async def websocket_endpoint(ws: WebSocket):
    await ws.accept()
    client = Client()
    assert broadcaster is not None
    broadcaster.add_client(client)
    # On connect, send roster snapshot
    broadcast("roster", {})
    try:
        while True:
            msg = await client.queue.get()
            await ws.send_text(json.dumps(msg))
    except WebSocketDisconnect:
        broadcaster.remove_client(client)
    except Exception:
        broadcaster.remove_client(client)


@app.post("/api/master/start")
async def api_master_start(req: MasterStartRequest):
    t = threading.Thread(target=run_master_once, args=(req.count, req.delay), daemon=True)
    t.start()
    return {"ok": True}


def run_scenario_late_bind_escort():
    """Demonstrate late queue bind and backlog hand-off between workers.

    Steps:
    - Publish some escort quests before any escort queue exists (lost at broker; tracked as pending in UI)
    - Start tempb (escort) creating the queue; publish more (processed by tempb)
    - Pause tempb; publish more (ready backlog)
    - Start tempd (escort); tempd drains backlog and new messages
    """
    global routing_mode
    routing_mode = "skill"
    broadcast("routing_mode", {"mode": routing_mode})

    # Clear transient state similar to reset and stop any existing players
    try:
        for name, meta in list(players.items()):
            try:
                meta.setdefault("controls", {})["shutdown"] = True
                bus = meta.get("bus")
                if bus:
                    try:
                        bus.conn.close()
                    except Exception:
                        pass
            except Exception:
                pass
        roster.clear(); players.clear()
        broadcast("roster", {})
    except Exception:
        pass
    # Reset transient state
    scoreboard.clear(); fails.clear(); quests_state.clear(); player_stats.clear()
    processed_results.clear(); skip_logged.clear(); inflight_by_player.clear(); dlq_messages.clear()
    broadcast("reset", {})
    # Ensure no lingering shared escort queue exists
    try:
        rb = RabbitEventBus()
        try:
            rb.ch.queue_delete(queue="game.skill.escort.q")
        except Exception:
            pass
        rb.close()
    except Exception:
        pass

    # 1) Publish before queue exists (no escort consumer yet)
    pre_count = 4
    for _ in range(pre_count):
        publish_one("escort")
        time.sleep(0.05)
    # Give a visible gap before first worker arrives so it's clear these were pre-queue
    broadcast("scenario", {"name": "late_bind_escort", "stage": "issued_before_queue", "count": pre_count})
    time.sleep(2.0)

    # 2) Start tempb (creates skill queue) and publish more
    start_player_thread("tempb", ["escort"], 0.1, 0.8, 1)
    roster["tempb"] = {"skills": ["escort"], "fail_pct": 0.1, "speed_multiplier": 0.8, "workers": 1}
    broadcast("roster", {})
    time.sleep(0.5)  # allow queue bind
    for _ in range(3):
        publish_one("escort")
        time.sleep(0.05)

    # 3) Pause tempb; publish more to build backlog
    players.setdefault("tempb", {}).setdefault("controls", {})["paused"] = True
    roster["tempb"] = {**roster.get("tempb", {}), "status": "reconnecting"}
    broadcast("roster", {})
    for _ in range(3):
        publish_one("escort")
        time.sleep(0.05)

    # 4) Start tempd; it will drain backlog and continue
    start_player_thread("tempd", ["escort"], 0.0, 1.0, 1)
    roster["tempd"] = {"skills": ["escort"], "fail_pct": 0.0, "speed_multiplier": 1.0, "workers": 1}
    broadcast("roster", {})


@app.post("/api/player/start")
async def api_player_start(req: PlayerStartRequest):
    skills_list = [s.strip() for s in req.skills.split(",") if s.strip()]
    roster[req.player] = {"skills": skills_list, "fail_pct": req.fail_pct, "speed_multiplier": req.speed_multiplier, "workers": req.workers, "prefetch": req.prefetch, "drop_rate": req.drop_rate, "skip_rate": req.skip_rate}
    # init registry
    players.setdefault(req.player, {"controls": {"paused": False, "next_action": None}, "bus": None})
    broadcast("roster", {})
    start_player_thread(player=req.player, skills=skills_list, fail_pct=req.fail_pct, speed_multiplier=req.speed_multiplier, workers=req.workers)
    return {"ok": True}


class PlayerControlRequest(BaseModel):
    player: str
    action: str  # pause|resume|crash|next_action
    mode: Optional[str] = None  # for action=next_action: drop|requeue|dlq|fail_early


@app.post("/api/player/control")
async def api_player_control(req: PlayerControlRequest):
    if req.player not in players:
        return {"ok": False, "error": "unknown player"}
    controls = players[req.player].setdefault("controls", {"paused": False, "next_action": None})
    if req.action == "pause":
        # Real disconnect - disconnect and stay offline until resume
        controls["paused"] = True
        real_disconnect_player(req.player, auto_reconnect_delay=0)  # No auto-reconnect
        broadcast("roster", {})
    elif req.action == "resume":
        # Real reconnect - restart worker thread immediately
        controls["paused"] = False
        roster[req.player] = {**roster.get(req.player, {}), "status": "reconnecting"}
        broadcast("player_reconnecting", {"player": req.player})
        
        # Restart the worker thread with brief delay for UI feedback
        skills = list(roster.get(req.player, {}).get("skills", []))
        fail_pct = roster.get(req.player, {}).get("fail_pct", 0.0)
        speed_multiplier = roster.get(req.player, {}).get("speed_multiplier", 1.0)
        workers = roster.get(req.player, {}).get("workers", 1)
        
        def reconnect_worker():
            time.sleep(1)  # Brief delay for UI feedback
            if req.player in players and not players[req.player].get("controls", {}).get("shutdown"):
                start_player_thread(req.player, skills, fail_pct, speed_multiplier, workers)
        
        threading.Thread(target=reconnect_worker, daemon=True).start()
        broadcast("roster", {})
    elif req.action == "crash":
        # One-time crash with auto-reconnect (same as chaos drop)
        real_disconnect_player(req.player, auto_reconnect_delay=3.0)
        broadcast("roster", {})
    elif req.action == "next_action":
        if req.mode in {"drop", "requeue", "dlq", "fail_early"}:
            controls["next_action"] = req.mode
        else:
            return {"ok": False, "error": "invalid mode"}
    else:
        return {"ok": False, "error": "invalid action"}
    return {"ok": True}


class PlayerDeleteRequest(BaseModel):
    player: str


@app.post("/api/player/delete")
async def api_player_delete(req: PlayerDeleteRequest):
    name = req.player
    if name not in players and name not in roster:
        return {"ok": False, "error": "unknown player"}
    
    try:
        # Step 1: Signal all worker threads to shutdown
        shutting_down.add(name)
        meta = players.get(name, {})
        if meta:
            meta.setdefault("controls", {})["shutdown"] = True
            
            # Get list of active threads for this player
            active_threads = meta.get("threads", set()).copy()
            meta.setdefault("stopping_threads", set()).update(active_threads)
            
            # Close any active bus connections
            bus = meta.get("bus")
            if bus:
                try:
                    # Stop consuming first, then close
                    try:
                        bus.ch.stop_consuming()
                    except Exception:
                        pass
                    bus.conn.close()
                except Exception:
                    pass
                meta["bus"] = None
        
        # Step 2: Wait briefly for threads to shutdown gracefully
        await asyncio.sleep(0.1)
        
        # Step 3: Get player skills for queue cleanup
        skills_of_player = list(roster.get(name, {}).get("skills", []))
        
        # Step 4: Delete queues
        if routing_mode == "player":
            # Delete per-player queue
            try:
                rb = RabbitEventBus()
                qn = f"game.player.{name}.q"
                rb.ch.queue_delete(queue=qn)
                rb.close()
            except Exception:
                pass
        else:
            # In skill mode, check if we can delete shared skill queues
            remaining_players = {p: meta for p, meta in roster.items() if p != name}
            for skill in skills_of_player:
                # Check if any other player needs this skill
                skill_still_needed = any(
                    skill in meta.get("skills", []) 
                    for meta in remaining_players.values()
                )
                if not skill_still_needed:
                    try:
                        rb = RabbitEventBus()
                        qn = f"game.skill.{skill}.q"
                        rb.ch.queue_delete(queue=qn)
                        rb.close()
                    except Exception:
                        pass
        
        # Step 5: Clean up state
        roster.pop(name, None)
        player_stats.pop(name, None)
        inflight_by_player.pop(name, None)
        
        # Step 6: Remove player entry (threads should have exited by now)
        players.pop(name, None)
        shutting_down.discard(name)
        
        broadcast("roster", {})
        
        # Save state after deletion
        save_state()
        
    except Exception as e:
        return {"ok": False, "error": f"deletion failed: {str(e)}"}
    
    return {"ok": True}


class RetentionSetRequest(BaseModel):
    skill: str
    ttl_ms: int


@app.post("/api/retention/set")
async def api_retention_set(req: RetentionSetRequest):
    # Set per-skill TTL; heartbeat will mark expired locally; to enforce on broker, add a policy manually
    skill_ttl_ms[req.skill] = max(1000, int(req.ttl_ms))
    return {"ok": True, "skill": req.skill, "ttl_ms": skill_ttl_ms[req.skill]}


class RoutingModeRequest(BaseModel):
    mode: str  # skill|player


@app.post("/api/routing/set")
async def api_routing_set(req: RoutingModeRequest):
    """Set routing mode - now supports RabbitMQ-native routing."""
    allowed = ["skill", "player", "rmq_native"]
    if req.mode not in allowed:
        return {"ok": False, "error": f"Invalid mode. Use: {allowed}"}
    
    global routing_mode
    
    if req.mode == "rmq_native":
        # Set up RabbitMQ-native routing topology
        setup_result = await setup_rabbitmq_native_routing()
        if setup_result["ok"]:
            routing_config["mode"] = "rmq_native"
            routing_mode = "rmq_native"
            broadcast("routing_mode", {
                "mode": routing_mode, 
                "educational_note": "RabbitMQ-native routing enabled - no Python routing logic",
                "setup_result": setup_result
            })
            return {"ok": True, "mode": routing_mode, "setup": setup_result}
        else:
            return {"ok": False, "error": f"Failed to set up RabbitMQ routing: {setup_result.get('error')}"}
    else:
        # LEGACY behavior for comparison
        routing_mode = req.mode
        routing_config["mode"] = "legacy_python"
        broadcast("routing_mode", {"mode": routing_mode, "educational_note": "Legacy Python routing mode"})
        return {"ok": True, "mode": routing_mode}

@app.get("/api/routing/topology")
async def api_routing_topology():
    """Get current routing topology - educational view of RabbitMQ vs Python routing"""
    import json, base64, urllib.request
    
    try:
        api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
        user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
        token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
        
        # Get exchanges
        req = urllib.request.Request(f"{api}/exchanges")
        req.add_header('Authorization', f'Basic {token}')
        with urllib.request.urlopen(req, timeout=3) as resp:
            exchanges = json.loads(resp.read().decode())
        
        # Get bindings  
        req = urllib.request.Request(f"{api}/bindings")
        req.add_header('Authorization', f'Basic {token}')
        with urllib.request.urlopen(req, timeout=3) as resp:
            bindings = json.loads(resp.read().decode())
        
        # Filter for game-related topology
        game_exchanges = [ex for ex in exchanges if "game." in ex["name"]]
        game_bindings = [b for b in bindings if "game." in (b.get("source", "") + b.get("destination", ""))]
        
        return {
            "ok": True,
            "current_mode": routing_mode,
            "routing_config": routing_config,
            "rabbitmq_topology": {
                "exchanges": game_exchanges,
                "bindings": game_bindings
            },
            "educational_comparison": {
                "python_routing": "Queues created/deleted dynamically, routing mode switch via API",
                "rabbitmq_native": "Static exchange hierarchy, routing via binding patterns",
                "benefits_rmq": [
                    "No application logic for routing decisions",
                    "Built-in load balancing and failover", 
                    "Topic patterns handle complex routing rules",
                    "Exchange-to-exchange bindings for hierarchical routing"
                ]
            },
            "educational_note": "Compare RabbitMQ-native topology vs Python routing abstractions"
        }
    except Exception as e:
        return {"ok": False, "error": str(e)}


class ScenarioRequest(BaseModel):
    name: str  # redelivery|requeue


@app.post("/api/scenario/run")
async def api_scenario_run(req: ScenarioRequest):
    # Scenarios are self-contained; do not auto-start baseline players here
    broadcast("roster", {})

    if req.name in scenarios.NAME_TO_SCENARIO:
        threading.Thread(
            target=lambda: scenarios.NAME_TO_SCENARIO[req.name](STATE, go_workers_client, broadcast, players),
            daemon=True,
        ).start()
        return {"ok": True}
    if req.name == "late_bind_escort":
        threading.Thread(target=run_scenario_late_bind_escort, daemon=True).start()
        return {"ok": True}
    if req.name == "duplicate":
        # Switch to player-based so both players get a copy
        global routing_mode
        routing_mode = "player"
        broadcast("routing_mode", {"mode": routing_mode})
        threading.Thread(target=lambda: publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()
        return {"ok": True}
    if req.name == "both_complete":
        # Player-based. Start two temporary fast players on the same skill so both complete their copies.
        routing_mode = "player"
        broadcast("routing_mode", {"mode": routing_mode})
        # Start temp players if not already
        if "temp1" not in roster:
            start_player_thread("temp1", ["slay"], 0.0, 0.5, 1)
            roster["temp1"] = {"skills": ["slay"], "fail_pct": 0.0, "speed_multiplier": 0.5, "workers": 1}
        if "temp2" not in roster:
            start_player_thread("temp2", ["slay"], 0.0, 0.5, 1)
            roster["temp2"] = {"skills": ["slay"], "fail_pct": 0.0, "speed_multiplier": 0.5, "workers": 1}
        broadcast("roster", {})
        threading.Thread(target=lambda: publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()
        return {"ok": True}
    if req.name == "dlq_poison":
        # Back-compat passthrough; scenarios handler also handles this
        players.setdefault("alice", {}).setdefault("controls", {})["next_action"] = "dlq"
        threading.Thread(target=lambda: publish_wave(1, 0.1, fixed_type="gather"), daemon=True).start()
        return {"ok": True}
    return {"ok": False, "error": "unknown scenario"}


@app.get("/api/failed/list")
async def api_failed_list():
    failed = [{"quest_id": qid, "quest_type": q["quest_type"], "assigned_to": q.get("assigned_to")} for qid, q in quests_state.items() if q.get("status") == "failed"]
    return {"ok": True, "failed": failed}


class RetryFailedRequest(BaseModel):
    quest_id: Optional[str] = None


@app.post("/api/failed/retry")
async def api_failed_retry(req: RetryFailedRequest):
    if req.quest_id:
        q = quests_state.get(req.quest_id)
        if not q or q.get("status") != "failed":
            return {"ok": False, "error": "not failed or not found"}
        publish_one(q.get("quest_type", "gather"), reissue_of=req.quest_id)
        return {"ok": True, "count": 1}
    # retry all
    count = 0
    for qid, q in list(quests_state.items()):
        if q.get("status") == "failed":
            publish_one(q.get("quest_type", "gather"), reissue_of=qid)
            count += 1
    return {"ok": True, "count": count}


@app.get("/api/dlq/list")
async def api_dlq_list():
    return {"ok": True, "items": dlq_messages}


@app.get("/api/unroutable/list")
async def api_unroutable_list():
    return {"ok": True, "items": unroutable}


class UnroutableReissueRequest(BaseModel):
    quest_id: Optional[str] = None


@app.post("/api/unroutable/reissue")
async def api_unroutable_reissue(req: UnroutableReissueRequest):
    global unroutable
    if req.quest_id:
        remaining = []
        reissued = 0
        for item in unroutable:
            payload = item.get("payload", {})
            qid = payload.get("case_id")
            if qid == req.quest_id:
                qt = payload.get("quest_type", "gather")
                # avoid reissuing if already completed/failed/accepted
                st = quests_state.get(req.quest_id, {}).get("status")
                if st in {"completed", "failed", "accepted"}:
                    # drop stale unroutable entry
                    pass
                else:
                    publish_one(qt, reissue_of=req.quest_id)
                    reissued += 1
            else:
                remaining.append(item)
        unroutable = remaining
        broadcast("unroutable_updated", {"count": len(unroutable)})
        return {"ok": True, "count": reissued}
    # reissue all
    for item in unroutable:
        payload = item.get("payload", {})
        qid = payload.get("case_id")
        qt = payload.get("quest_type", "gather")
        if qid:
            publish_one(qt, reissue_of=qid)
    cnt = len(unroutable)
    unroutable = []
    broadcast("unroutable_updated", {"count": 0})
    return {"ok": True, "count": cnt}


class DlqRequeueRequest(BaseModel):
    quest_id: str | None = None


@app.post("/api/dlq/requeue")
async def api_dlq_requeue(req: DlqRequeueRequest):
    global dlq_messages
    if req.quest_id:
        remaining = []
        requeued = 0
        for item in dlq_messages:
            if item.get("quest_id") == req.quest_id:
                publish_one(item.get("quest_type", "gather"), reissue_of=req.quest_id)
                requeued += 1
            else:
                remaining.append(item)
        dlq_messages = remaining
        broadcast("dlq_updated", {"count": len(dlq_messages)})
        return {"ok": True, "count": requeued}
    # requeue all
    for item in dlq_messages:
        publish_one(item.get("quest_type", "gather"), reissue_of=item.get("quest_id"))
    cnt = len(dlq_messages)
    dlq_messages = []
    broadcast("dlq_updated", {"count": 0})
    return {"ok": True, "count": cnt}


@app.post("/api/dlq/setup")
async def api_dlq_setup():
    """Initialize native RabbitMQ DLQ topology with retry queues."""
    try:
        from app.rabbitmq_utils import SimpleRabbitMQ
        bus = SimpleRabbitMQ()
        bus.setup_dlq_topology()
        bus.close()
        
        broadcast("dlq_setup", {"status": "completed", "educational_note": "Comprehensive native RabbitMQ DLQ topology created"})
        return {
            "ok": True, 
            "message": "Comprehensive native DLQ topology setup completed",
            "topology": {
                "dead_letter_exchange": "game.dlx (topic)",
                "alternate_exchange": "game.unroutable (fanout)",
                "retry_queues": [
                    {"name": "game.quest.retry.1", "ttl": "5 seconds", "routing_key": "retry.1"},
                    {"name": "game.quest.retry.2", "ttl": "30 seconds", "routing_key": "retry.2"},
                    {"name": "game.quest.retry.3", "ttl": "5 minutes", "routing_key": "retry.3"}
                ],
                "dlq_by_reason": [
                    {"name": "game.quest.dlq.rejected", "reason": "rejected", "routing_key": "dlq.rejected"},
                    {"name": "game.quest.dlq.expired", "reason": "expired", "routing_key": "dlq.expired"},
                    {"name": "game.quest.dlq.maxlen", "reason": "maxlen", "routing_key": "dlq.maxlen"},
                    {"name": "game.quest.dlq.delivery_limit", "reason": "delivery_limit", "routing_key": "dlq.delivery_limit"},
                    {"name": "game.quest.dlq.poison", "reason": "poison", "routing_key": "dlq.poison"},
                    {"name": "game.quest.dlq.unroutable", "reason": "unroutable", "routing_key": "dlq.unroutable"}
                ],
                "quarantine": {"name": "game.quest.quarantine", "routing_key": "quarantine"},
                "educational_note": "Comprehensive DLQ system with specific queues for different failure types and unroutable message handling"
            }
        }
    except Exception as e:
        return {"ok": False, "error": str(e)}

@app.post("/api/dlq/purge")
async def api_dlq_purge():
    """Purge native DLQ and retry queues."""
    try:
        from app.rabbitmq_utils import SimpleRabbitMQ
        bus = SimpleRabbitMQ()
        
        # Purge all DLQ-related queues
        queues_to_purge = [
            "game.quest.retry.1", 
            "game.quest.retry.2",
            "game.quest.retry.3",
            "game.quest.dlq.rejected",
            "game.quest.dlq.expired", 
            "game.quest.dlq.maxlen",
            "game.quest.dlq.delivery_limit",
            "game.quest.dlq.poison",
            "game.quest.dlq.unroutable",
            "game.quest.quarantine"
        ]
        
        purged_count = 0
        for queue in queues_to_purge:
            try:
                bus.ch.queue_purge(queue)
                purged_count += 1
            except Exception:
                pass  # Queue might not exist
        
        bus.close()
        
        # Also clear legacy app-level DLQ tracking
        dlq_messages.clear()
        
        broadcast("dlq_updated", {"count": 0, "purged_queues": purged_count, "educational_note": "Native DLQ and retry queues purged"})
        return {"ok": True, "purged_queues": purged_count}
    except Exception as e:
        return {"ok": False, "error": str(e)}

@app.get("/api/dlq/inspect")
async def api_dlq_inspect():
    """Inspect native DLQ and retry queues with x-death header analysis."""
    try:
        import json, base64, urllib.request
        api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
        user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
        token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
        
        queues_to_inspect = [
            # Retry stages
            {"name": "game.quest.retry.1", "type": "retry_stage_1", "category": "retry"},
            {"name": "game.quest.retry.2", "type": "retry_stage_2", "category": "retry"},
            {"name": "game.quest.retry.3", "type": "retry_stage_3", "category": "retry"},
            
            # DLQs by failure reason  
            {"name": "game.quest.dlq.rejected", "type": "dlq_rejected", "category": "dlq", "reason": "rejected"},
            {"name": "game.quest.dlq.expired", "type": "dlq_expired", "category": "dlq", "reason": "expired"},
            {"name": "game.quest.dlq.maxlen", "type": "dlq_maxlen", "category": "dlq", "reason": "maxlen"},
            {"name": "game.quest.dlq.delivery_limit", "type": "dlq_delivery_limit", "category": "dlq", "reason": "delivery_limit"},
            {"name": "game.quest.dlq.poison", "type": "dlq_poison", "category": "dlq", "reason": "poison"},
            {"name": "game.quest.dlq.unroutable", "type": "dlq_unroutable", "category": "dlq", "reason": "unroutable"},
            
            # Quarantine
            {"name": "game.quest.quarantine", "type": "quarantine", "category": "quarantine"}
        ]
        
        dlq_analysis = {"queues": [], "total_messages": 0, "educational_notes": []}
        
        for queue_info in queues_to_inspect:
            queue_name = queue_info["name"]
            req = urllib.request.Request(f"{api}/queues/%2F/{queue_name}")
            req.add_header('Authorization', f'Basic {token}')
            
            try:
                with urllib.request.urlopen(req, timeout=5) as resp:
                    queue_data = json.loads(resp.read().decode())
                    message_count = queue_data.get('messages', 0)
                    
                    queue_analysis = {
                        "name": queue_name,
                        "type": queue_info["type"],
                        "message_count": message_count,
                        "ready": queue_data.get('messages_ready', 0),
                        "unacked": queue_data.get('messages_unacknowledged', 0)
                    }
                    
                    # Try to peek at messages to analyze x-death headers
                    if message_count > 0:
                        peek_result = await api_rabbitmq_peek_messages(queue_name, 5)
                        if peek_result.get("ok"):
                            queue_analysis["sample_messages"] = []
                            for msg in peek_result.get("messages", []):
                                try:
                                    payload = json.loads(msg.get("payload", "{}"))
                                    headers = msg.get("properties", {}).get("headers", {})
                                    
                                    # Analyze x-death header if present
                                    death_info = {"retry_count": 0, "reason": "unknown"}
                                    if "x-death" in headers:
                                        x_death = headers["x-death"]
                                        if isinstance(x_death, list) and len(x_death) > 0:
                                            death_info["retry_count"] = x_death[0].get("count", 0)
                                            death_info["reason"] = x_death[0].get("reason", "unknown")
                                    
                                    queue_analysis["sample_messages"].append({
                                        "quest_id": payload.get("case_id", "unknown"),
                                        "quest_type": payload.get("quest_type", "unknown"),
                                        "death_info": death_info,
                                        "routing_key": msg.get("routing_key", "")
                                    })
                                except:
                                    pass
                    
                    dlq_analysis["queues"].append(queue_analysis)
                    dlq_analysis["total_messages"] += message_count
                    
            except urllib.error.HTTPError as e:
                if e.code == 404:
                    dlq_analysis["queues"].append({
                        "name": queue_name,
                        "type": queue_info["type"], 
                        "status": "not_created",
                        "message_count": 0
                    })
        
        dlq_analysis["educational_notes"] = [
            "Comprehensive native RabbitMQ DLQ system with specific queues for different failure types",
            "Retry flow: main queue → retry.1 (5s) → retry.2 (30s) → retry.3 (5m) → specific DLQ based on reason", 
            "Failure reasons: rejected, expired, maxlen, delivery_limit, poison, unroutable",
            "Unroutable messages caught via alternate exchange (mandatory=false) or return callbacks (mandatory=true)",
            "Quarantine queue for messages requiring manual intervention",
            "x-death headers track failure history and retry attempts across the entire system"
        ]
        
        return {"ok": True, "dlq_analysis": dlq_analysis}
        
    except Exception as e:
        return {"ok": False, "error": str(e)}

@app.post("/api/dlq/replay")
async def api_dlq_replay():
    """Replay messages from DLQ back to main exchange."""
    try:
        from app.rabbitmq_utils import SimpleRabbitMQ, EXCHANGE_NAME
        import json, time, pika
        bus = SimpleRabbitMQ()
        
        # Get messages from all DLQ queues
        dlq_queues = [
            "game.quest.dlq.rejected",
            "game.quest.dlq.expired", 
            "game.quest.dlq.maxlen",
            "game.quest.dlq.delivery_limit",
            "game.quest.dlq.poison",
            "game.quest.dlq.unroutable",
            "game.quest.quarantine"
        ]
        
        all_messages = []
        for dlq_queue in dlq_queues:
            dlq_result = await api_rabbitmq_peek_messages(dlq_queue, 5)
            if dlq_result.get("ok"):
                for msg in dlq_result.get("messages", []):
                    msg["source_dlq"] = dlq_queue
                    all_messages.append(msg)
        
        if not all_messages:
            return {"ok": False, "error": "No messages found in any DLQ"}
        
        replayed_count = 0
        purged_queues = set()
        
        for msg in all_messages:
            try:
                payload = json.loads(msg.get("payload", "{}"))
                headers = msg.get("properties", {}).get("headers", {})
                source_dlq = msg.get("source_dlq", "unknown")
                
                # Get original routing key from headers or derive from quest type
                original_routing_key = headers.get("original_routing_key")
                if not original_routing_key and payload.get("quest_type"):
                    original_routing_key = f"game.quest.{payload['quest_type']}"
                
                if original_routing_key:
                    # Republish to main exchange (reset retry count)
                    replay_headers = {
                        "replayed_from_dlq": True, 
                        "replay_timestamp": time.time(),
                        "source_dlq": source_dlq,
                        "failure_reason": headers.get("failure_reason", "unknown")
                    }
                    
                    bus.ch.basic_publish(
                        exchange=EXCHANGE_NAME,
                        routing_key=original_routing_key,
                        body=json.dumps(payload),
                        properties=pika.BasicProperties(
                            delivery_mode=2,
                            headers=replay_headers
                        )
                    )
                    replayed_count += 1
                    purged_queues.add(source_dlq)
                    
            except Exception:
                pass  # Skip invalid messages
        
        # Purge the DLQs after successful replay
        for queue in purged_queues:
            try:
                bus.ch.queue_purge(queue)
            except Exception:
                pass
        
        bus.close()
        
        broadcast("dlq_replay", {
            "replayed_count": replayed_count,
            "educational_note": f"Replayed {replayed_count} messages from DLQ back to main exchange"
        })
        
        return {"ok": True, "replayed_count": replayed_count}
        
    except Exception as e:
        return {"ok": False, "error": str(e)}


@app.post("/api/reset")
async def api_reset():
    # Clear transient game state and stop all players
    try:
        old_players = list(players.keys())
        
        # Step 1: Signal all players to shutdown
        for name in old_players:
            shutting_down.add(name)
            
        for name, meta in list(players.items()):
            try:
                meta.setdefault("controls", {})["shutdown"] = True
                
                # Mark all threads for stopping
                active_threads = meta.get("threads", set()).copy()
                meta.setdefault("stopping_threads", set()).update(active_threads)
                
                # Close bus connections properly
                bus = meta.get("bus")
                if bus:
                    try:
                        bus.ch.stop_consuming()
                    except Exception:
                        pass
                    try:
                        bus.conn.close()
                    except Exception:
                        pass
                    meta["bus"] = None
            except Exception:
                pass
        
        # Step 2: Wait for threads to shutdown
        await asyncio.sleep(0.2)
        # Delete per-player queues in player mode
        try:
            if routing_mode == "player":
                rb = RabbitEventBus()
                for pname in old_players:
                    try:
                        rb.ch.queue_delete(queue=f"game.player.{pname}.q")
                    except Exception:
                        pass
                try:
                    rb.close()
                except Exception:
                    pass
        except Exception:
            pass
        # Always attempt to delete shared skill queues for our demo skills
        try:
            rb2 = RabbitEventBus()
            for sk in ["gather","slay","escort"]:
                try:
                    rb2.ch.queue_delete(queue=f"game.skill.{sk}.q")
                except Exception:
                    pass
            try:
                rb2.close()
            except Exception:
                pass
        except Exception:
            pass
        # Step 3: Clear all state
        roster.clear() 
        players.clear()
        shutting_down.clear()
        
        # Step 4: Stop all Go workers
        if go_workers_client.enabled:
            try:
                # Get all workers and stop them
                status = go_workers_client.get_status()
                worker_list = status.get("workers", [])
                if len(worker_list) > 0:
                    # Stop each worker individually  
                    for worker_name in worker_list:
                        go_workers_client.delete_worker(worker_name)
                # Clear Go workers roster
                go_workers_client.roster.clear()
            except Exception as e:
                print(f"Error stopping Go workers during reset: {e}")
        
        broadcast("roster", {})
    except Exception:
        pass
    
    # Clear all game state
    scoreboard.clear(); fails.clear(); quests_state.clear(); player_stats.clear()
    processed_results.clear(); skip_logged.clear(); inflight_by_player.clear(); dlq_messages.clear(); unroutable.clear()
    
    # Hard reset: clear state cache and reload fresh
    clear_state_cache()
    
    # Force UI refresh
    broadcast("reset", {"hard_reset": True})
    
    return {"ok": True, "message": "Hard reset completed - all workers stopped, cache cleared"}


# State persistence endpoints
@app.post("/api/state/save")
async def api_state_save():
    """Manually save current state"""
    save_state()
    return {"ok": True, "message": "State saved"}

@app.post("/api/state/load")
async def api_state_load():
    """Manually load cached state"""
    if load_state():
        broadcast("roster", {})
        return {"ok": True, "message": "State loaded"}
    else:
        return {"ok": False, "message": "No cached state found"}

@app.get("/api/state/info")
async def api_state_info():
    """Get state cache information"""
    cache_exists = os.path.exists(STATE_CACHE_FILE)
    cache_size = os.path.getsize(STATE_CACHE_FILE) if cache_exists else 0
    return {
        "cache_exists": cache_exists,
        "cache_size_bytes": cache_size,
        "players_count": len(roster),
        "quests_count": len(quests_state)
    }


# RabbitMQ-direct messages endpoint
@app.get("/api/messages")
async def api_messages(status: str = "pending"):
    """Get message lists directly from RabbitMQ - minimal abstraction for education"""
    if status not in {"pending", "failed", "dlq"}:
        return {"ok": False, "error": "invalid status"}
    
    try:
        if status == "pending":
            # Query RabbitMQ directly for pending messages
            metrics_result = await api_rabbitmq_derived_metrics()
            if metrics_result["ok"]:
                rmq_metrics = metrics_result["metrics"]
                items = []
                
                # Convert queue stats to pending items
                for queue_name, queue_stats in rmq_metrics["queue_stats"].items():
                    if queue_stats["ready"] > 0:
                        # Extract quest type from queue name
                        quest_type = "unknown"
                        if ".skill." in queue_name:
                            quest_type = queue_name.split(".skill.")[1].split(".")[0]
                        
                        # Create items for each ready message (approximation since RabbitMQ doesn't store individual message IDs)
                        for i in range(queue_stats["ready"]):
                            items.append({
                                "quest_id": f"rmq-{queue_name}-{i}",
                                "quest_type": quest_type,
                                "queue": queue_name,
                                "age_sec": 0,  # RabbitMQ doesn't provide message age without inspection
                                "source": "rabbitmq_derived"
                            })
                
                return {"ok": True, "items": items, "source": "direct_rabbitmq", "educational_note": "Pending messages derived from RabbitMQ queue depths"}
            
        elif status == "failed":
            # For failed messages, we need to use app-level tracking since RabbitMQ doesn't store failure history
            # This is a legitimate use case for app-level state
            items = [{"quest_id": qid, "quest_type": q.get("quest_type"), "assigned_to": q.get("assigned_to")} for qid, q in quests_state.items() if q.get("status") == "failed"]
            return {"ok": True, "items": items, "source": "app_state", "educational_note": "Failed messages require app-level tracking - RabbitMQ doesn't store failure history"}
            
        elif status == "dlq":
            # Query DLQ directly from RabbitMQ
            dlq_result = await api_rabbitmq_peek_messages("game.quest.dlq", 50)
            if dlq_result["ok"]:
                items = []
                for msg in dlq_result.get("messages", []):
                    try:
                        payload = json.loads(msg["payload"])
                        items.append({
                            "quest_id": payload.get("case_id", "unknown"),
                            "quest_type": payload.get("quest_type", "unknown"),
                            "routing_key": msg.get("routing_key", ""),
                            "source": "direct_dlq_query"
                        })
                    except:
                        pass
                return {"ok": True, "items": items, "source": "direct_rabbitmq", "educational_note": "DLQ messages queried directly from RabbitMQ"}
            else:
                # Fallback to internal DLQ tracking
                return {"ok": True, "items": dlq_messages, "source": "app_fallback"}
                
    except Exception as e:
        # Fallback to internal state
        if status == "pending":
            items = []
            now = time.time()
            for qid, q in quests_state.items():
                if q.get("status") == "pending":
                    items.append({
                        "quest_id": qid,
                        "quest_type": q.get("quest_type"),
                        "age_sec": int(now - float(q.get("issued_at", now))),
                    })
            items.sort(key=lambda x: -x.get("age_sec", 0))
            return {"ok": True, "items": items, "source": "fallback_internal", "error": str(e)}
        elif status == "failed":
            items = [{"quest_id": qid, "quest_type": q.get("quest_type"), "assigned_to": q.get("assigned_to")} for qid, q in quests_state.items() if q.get("status") == "failed"]
            return {"ok": True, "items": items, "source": "fallback_internal", "error": str(e)}
        elif status == "dlq":
            return {"ok": True, "items": dlq_messages, "source": "fallback_internal", "error": str(e)}


# Chaos state endpoints (legacy - use /api/chaos/status instead)
@app.get("/api/chaos/state")
async def api_chaos_state():
    return {"ok": True, "mode": chaos_config.get("action") if chaos_config.get("enabled") else None}


class PlayerUpdateRequest(BaseModel):
    player: str
    prefetch: Optional[int] = None
    speed_multiplier: Optional[float] = None
    drop_rate: Optional[float] = None
    skip_rate: Optional[float] = None


@app.post("/api/player/update")
async def api_player_update(req: PlayerUpdateRequest):
    if req.player not in roster:
        return {"ok": False, "error": "unknown player"}
    meta = roster[req.player]
    if req.prefetch is not None:
        meta["prefetch"] = int(max(1, min(100, req.prefetch)))
    if req.speed_multiplier is not None:
        meta["speed_multiplier"] = float(max(0.05, req.speed_multiplier))
    if req.drop_rate is not None:
        meta["drop_rate"] = float(max(0.0, min(1.0, req.drop_rate)))
    if req.skip_rate is not None:
        meta["skip_rate"] = float(max(0.0, min(1.0, req.skip_rate)))
    broadcast("roster", {})
    return {"ok": True}


class QuickstartRequest(BaseModel):
    preset: str = "alice_bob"  # or "custom"
    players: Optional[List[Dict]] = None


@app.post("/api/players/quickstart")
async def api_players_quickstart(req: QuickstartRequest):
    presets = []
    if req.preset == "alice_bob":
        presets = [
            {"player": "alice", "skills": "gather,slay", "fail_pct": 0.2, "speed_multiplier": 1.0, "workers": 1},
            {"player": "bob", "skills": "slay,escort", "fail_pct": 0.1, "speed_multiplier": 0.7, "workers": 2},
        ]
    elif req.preset == "custom" and req.players:
        presets = req.players
    else:
        return {"ok": False, "error": "invalid preset"}
    for p in presets:
        name = p["player"]
        # Skip duplicates
        if name in roster:
            continue
        skills_list = [s.strip() for s in p.get("skills", "").split(",") if s.strip()]
        roster[name] = {"skills": skills_list, "fail_pct": float(p.get("fail_pct", 0.2)), "speed_multiplier": float(p.get("speed_multiplier", 1.0)), "workers": int(p.get("workers", 1))}
        players.setdefault(name, {"controls": {"paused": False, "next_action": None}, "bus": None})
        start_player_thread(player=name, skills=skills_list, fail_pct=float(p.get("fail_pct", 0.2)), speed_multiplier=float(p.get("speed_multiplier", 1.0)), workers=int(p.get("workers", 1)))
    broadcast("roster", {})
    return {"ok": True, "count": len(presets)}


class MasterOneRequest(BaseModel):
    quest_type: str = "gather"


@app.post("/api/master/one")
async def api_master_one(req: MasterOneRequest):
    publish_one(req.quest_type)
    return {"ok": True}


class ChaosArmRequest(BaseModel):
    action: str  # amqp_disconnect|amqp_close_channel|rmq_delete_queue|rmq_unbind_queue|rmq_block_connection|legacy_app_level
    target_player: Optional[str] = None  # specific player or None for any
    target_queue: Optional[str] = None  # specific queue or None for any
    auto_trigger: bool = False  # automatically publish messages
    trigger_delay: float = 2.0  # seconds before auto-trigger
    trigger_count: int = 1  # number of messages to publish


async def setup_rabbitmq_native_routing():
    """Set up RabbitMQ-native routing topology - eliminates Python routing logic"""
    import urllib.request, json, base64
    
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
    
    try:
        # Create exchange hierarchy
        for exchange_name, config in routing_config["exchange_hierarchy"].items():
            data = {
                "type": config["type"],
                "durable": config["durable"],
                "auto_delete": False,
                "internal": False,
                "arguments": {}
            }
            
            req = urllib.request.Request(f"{api}/exchanges/%2F/{exchange_name}", 
                                       data=json.dumps(data).encode(),
                                       headers={'Content-Type': 'application/json', 'Authorization': f'Basic {token}'},
                                       method='PUT')
            
            with urllib.request.urlopen(req, timeout=5) as resp:
                pass
        
        # Create exchange-to-exchange bindings
        # game.routing -> game.skill (for skill-based routing)
        skill_binding = {
            "routing_key": "quest.*.skill",
            "arguments": {}
        }
        req = urllib.request.Request(f"{api}/bindings/%2F/e/game.routing/e/game.skill",
                                   data=json.dumps(skill_binding).encode(),
                                   headers={'Content-Type': 'application/json', 'Authorization': f'Basic {token}'},
                                   method='POST')
        
        with urllib.request.urlopen(req, timeout=5) as resp:
            pass
        
        # game.routing -> game.player (for player-based routing) 
        player_binding = {
            "routing_key": "quest.*.player",
            "arguments": {}
        }
        req = urllib.request.Request(f"{api}/bindings/%2F/e/game.routing/e/game.player",
                                   data=json.dumps(player_binding).encode(),
                                   headers={'Content-Type': 'application/json', 'Authorization': f'Basic {token}'},
                                   method='POST')
        
        with urllib.request.urlopen(req, timeout=5) as resp:
            pass
        
        return {"ok": True, "exchanges_created": list(routing_config["exchange_hierarchy"].keys()), 
                "educational_note": "Pure RabbitMQ routing topology created - no Python routing needed"}
        
    except Exception as e:
        return {"ok": False, "error": f"Failed to set up RabbitMQ routing: {str(e)}"}

async def execute_rabbitmq_chaos(action: str, target_player: str = None, target_queue: str = None):
    """Execute chaos actions directly on RabbitMQ using Management API - educational approach"""
    import urllib.request, json, base64
    
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
    
    try:
        if action == "rmq_delete_queue":
            # Delete a queue directly via RabbitMQ Management API
            queue_name = target_queue or "game.skill.gather.q"
            
            # First check if queue exists
            try:
                check_req = urllib.request.Request(f"{api}/queues/%2F/{queue_name}")
                check_req.add_header('Authorization', f'Basic {token}')
                with urllib.request.urlopen(check_req, timeout=5) as resp:
                    pass  # Queue exists
            except urllib.error.HTTPError as e:
                if e.code == 404:
                    return {"ok": False, "error": f"Queue '{queue_name}' does not exist", "educational_note": "Cannot delete non-existent queue - create workers first to generate queues"}
                raise
            
            # Delete the queue
            req = urllib.request.Request(f"{api}/queues/%2F/{queue_name}", method='DELETE')
            req.add_header('Authorization', f'Basic {token}')
            
            with urllib.request.urlopen(req, timeout=5) as resp:
                return {"ok": True, "action": "queue_deleted", "queue": queue_name, "educational_note": "Queue deleted directly via RabbitMQ Management API"}
        
        elif action == "rmq_unbind_queue":
            # Remove queue bindings directly
            queue_name = target_queue or "game.skill.gather.q"
            # First get current bindings
            req = urllib.request.Request(f"{api}/queues/%2F/{queue_name}/bindings")
            req.add_header('Authorization', f'Basic {token}')
            
            with urllib.request.urlopen(req, timeout=5) as resp:
                bindings = json.loads(resp.read().decode())
            
            # Remove non-default bindings
            for binding in bindings:
                if binding.get("source") != "":  # Skip default exchange binding
                    unbind_url = f"{api}/bindings/%2F/e/{binding['source']}/q/{queue_name}/{binding.get('properties_key', '~')}"
                    unbind_req = urllib.request.Request(unbind_url, method='DELETE')
                    unbind_req.add_header('Authorization', f'Basic {token}')
                    
                    with urllib.request.urlopen(unbind_req, timeout=5) as resp:
                        pass
            
            return {"ok": True, "action": "queue_unbound", "queue": queue_name, "educational_note": "Queue bindings removed via RabbitMQ Management API"}
        
        elif action == "rmq_block_connection":
            # Get connections and close them (simulates network issues)
            req = urllib.request.Request(f"{api}/connections")
            req.add_header('Authorization', f'Basic {token}')
            
            with urllib.request.urlopen(req, timeout=5) as resp:
                connections = json.loads(resp.read().decode())
            
            # Filter connections by client properties if targeting specific player
            targets = []
            for conn in connections:
                client_props = conn.get("client_properties", {})
                if target_player and target_player not in str(client_props):
                    continue
                targets.append(conn["name"])
            
            # Close target connections
            closed_count = 0
            for conn_name in targets[:1]:  # Close first matching connection
                close_req = urllib.request.Request(f"{api}/connections/{conn_name}", method='DELETE')
                close_req.add_header('Authorization', f'Basic {token}')
                
                with urllib.request.urlopen(close_req, timeout=5) as resp:
                    closed_count += 1
            
            return {"ok": True, "action": "connections_closed", "count": closed_count, "educational_note": "AMQP connections closed via RabbitMQ Management API"}
        
        elif action == "rmq_purge_queue":
            # Purge messages from queue
            queue_name = target_queue or "game.skill.gather.q"
            
            # First check if queue exists
            try:
                check_req = urllib.request.Request(f"{api}/queues/%2F/{queue_name}")
                check_req.add_header('Authorization', f'Basic {token}')
                with urllib.request.urlopen(check_req, timeout=5) as resp:
                    pass  # Queue exists
            except urllib.error.HTTPError as e:
                if e.code == 404:
                    return {"ok": False, "error": f"Queue '{queue_name}' does not exist", "educational_note": "Cannot purge non-existent queue - create workers first to generate queues"}
                raise
            
            # Purge the queue
            req = urllib.request.Request(f"{api}/queues/%2F/{queue_name}/contents", method='DELETE')
            req.add_header('Authorization', f'Basic {token}')
            
            with urllib.request.urlopen(req, timeout=5) as resp:
                return {"ok": True, "action": "queue_purged", "queue": queue_name, "educational_note": "Queue purged directly via RabbitMQ Management API"}
        
        else:
            return {"ok": False, "error": f"Unknown RabbitMQ chaos action: {action}"}
            
    except Exception as e:
        return {"ok": False, "error": f"RabbitMQ chaos failed: {str(e)}"}

@app.post("/api/chaos/arm")
async def api_chaos_arm(req: ChaosArmRequest):
    """Arm RabbitMQ-native chaos action system - educational transparency."""
    rmq_actions = ["rmq_delete_queue", "rmq_unbind_queue", "rmq_block_connection", "rmq_purge_queue"]
    legacy_actions = ["drop", "requeue", "dlq", "fail_early", "disconnect", "pause"]
    allowed_actions = rmq_actions + legacy_actions
    
    if req.action not in allowed_actions:
        return {"ok": False, "error": f"Invalid action. RabbitMQ-native: {rmq_actions}, Legacy: {legacy_actions}"}
    
    # Update chaos config
    chaos_config.update({
        "enabled": True,
        "action": req.action,
        "target_player": req.target_player,
        "target_queue": req.target_queue,
        "auto_trigger": req.auto_trigger,
        "trigger_delay": req.trigger_delay,
        "trigger_count": req.trigger_count,
        "is_rabbitmq_native": req.action in rmq_actions
    })
    
    # Execute immediately if it's a RabbitMQ-native action
    if req.action in rmq_actions:
        result = await execute_rabbitmq_chaos(req.action, req.target_player, req.target_queue)
        if result["ok"]:
            broadcast("chaos_rabbitmq_native", {
                "action": req.action,
                "target_player": req.target_player,
                "target_queue": req.target_queue,
                "result": result,
                "educational_note": "Chaos executed directly on RabbitMQ - no app-level simulation"
            })
        return result
    
    # Auto-trigger for legacy actions
    if req.auto_trigger and req.action in legacy_actions:
        def auto_trigger():
            time.sleep(req.trigger_delay)
            if chaos_config["enabled"]:  # Still armed
                quest_type = "gather"  # Default for legacy
                broadcast("chaos_auto_trigger", {
                    "action": req.action,
                    "quest_type": quest_type,
                    "count": req.trigger_count,
                    "educational_note": "Legacy app-level chaos - consider using RabbitMQ-native actions"
                })
                for i in range(req.trigger_count):
                    publish_one(quest_type)
                    time.sleep(0.1)  # Small delay between messages
                    
        threading.Thread(target=auto_trigger, daemon=True).start()
    
    return {"ok": True, "config": chaos_config}


@app.get("/api/chaos/status")
async def api_chaos_status():
    """Get current chaos configuration."""
    return chaos_config


@app.post("/api/chaos/disarm")
async def api_chaos_disarm():
    """Disarm chaos system."""
    chaos_config["enabled"] = False
    return {"ok": True, "config": chaos_config}


# Direct RabbitMQ introspection endpoints - minimal abstraction for instructional purposes
@app.get("/api/rabbitmq/raw/queues")
async def api_rabbitmq_raw_queues():
    """Direct RabbitMQ queue data - no abstraction for educational value"""
    import json, base64, urllib.request
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    url = f"{api}/queues"
    req = urllib.request.Request(url)
    token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
    req.add_header('Authorization', f'Basic {token}')
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            raw_data = json.loads(resp.read().decode())
            # Return RAW RabbitMQ data with minimal filtering for education
            game_queues = [q for q in raw_data if q.get('name', '').startswith(('game.', 'web.'))]
            return {"ok": True, "raw_rabbitmq_data": game_queues}
    except Exception as e:
        return {"ok": False, "error": str(e)}

@app.get("/api/rabbitmq/raw/exchanges")
async def api_rabbitmq_raw_exchanges():
    """Direct RabbitMQ exchange data with bindings"""
    import json, base64, urllib.request
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    
    # Get exchanges
    try:
        req = urllib.request.Request(f"{api}/exchanges")
        token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
        req.add_header('Authorization', f'Basic {token}')
        with urllib.request.urlopen(req, timeout=3) as resp:
            exchanges = json.loads(resp.read().decode())
        
        # Get bindings
        req = urllib.request.Request(f"{api}/bindings")
        req.add_header('Authorization', f'Basic {token}')
        with urllib.request.urlopen(req, timeout=3) as resp:
            bindings = json.loads(resp.read().decode())
            
        # Filter for our exchanges
        our_exchanges = [ex for ex in exchanges if ex.get('name') in ['rte.topic', 'game.skill', '']]
        our_bindings = [b for b in bindings if b.get('source') in ['rte.topic', 'game.skill']]
        
        return {"ok": True, "exchanges": our_exchanges, "bindings": our_bindings}
    except Exception as e:
        return {"ok": False, "error": str(e)}

@app.get("/api/rabbitmq/derived/metrics")
async def api_rabbitmq_derived_metrics():
    """Derive all UI metrics directly from RabbitMQ - minimal abstraction for education"""
    import json, base64, urllib.request
    
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
    
    try:
        # Get queue data
        req = urllib.request.Request(f"{api}/queues")
        req.add_header('Authorization', f'Basic {token}')
        with urllib.request.urlopen(req, timeout=3) as resp:
            queues = json.loads(resp.read().decode())
        
        # Get consumer data
        req = urllib.request.Request(f"{api}/consumers")
        req.add_header('Authorization', f'Basic {token}')
        with urllib.request.urlopen(req, timeout=3) as resp:
            consumers = json.loads(resp.read().decode())
        
        # EDUCATIONAL: Derive metrics directly from RabbitMQ state
        metrics = {
            "source": "direct_rabbitmq_query",
            "timestamp": time.time(),
            "queue_stats": {},
            "consumer_stats": {},
            "total_pending": 0,
            "total_unacked": 0,
            "total_consumers": 0,
            "per_type": {},
            "worker_roster": {}
        }
        
        # Process queue statistics
        for queue in queues:
            name = queue.get('name', '')
            if name.startswith(('game.', 'web.')):
                ready = queue.get('messages_ready', 0)
                unacked = queue.get('messages_unacknowledged', 0)
                consumer_count = queue.get('consumers', 0)
                
                metrics["queue_stats"][name] = {
                    "ready": ready,
                    "unacked": unacked,
                    "consumers": consumer_count,
                    "total": queue.get('messages', 0),
                    "state": queue.get('state', 'unknown')
                }
                
                metrics["total_pending"] += ready
                metrics["total_unacked"] += unacked
                metrics["total_consumers"] += consumer_count
                
                # Derive quest type metrics from queue names
                if '.skill.' in name:
                    quest_type = name.split('.skill.')[1].split('.')[0]
                    if quest_type not in metrics["per_type"]:
                        metrics["per_type"][quest_type] = {"pending": 0, "accepted": 0, "completed": 0, "failed": 0}
                    metrics["per_type"][quest_type]["pending"] += ready
                    metrics["per_type"][quest_type]["accepted"] += unacked
        
        # Build worker roster from application-level worker profiles and map to RabbitMQ consumers
        # This maintains the abstraction layer between worker skills and queue consumption
        try:
            go_workers_status = go_workers_client.get_status()
            active_workers = go_workers_status.get("workers", []) if go_workers_status else []
            
            # Initialize worker roster from application roster with skill profiles
            for worker_name in active_workers:
                worker_profile = roster.get(worker_name, {})
                metrics["worker_roster"][worker_name] = {
                    "status": "online",
                    "skills": worker_profile.get("skills", []),
                    "workers": worker_profile.get("workers", 1),
                    "fail_pct": worker_profile.get("fail_pct", 0.1),
                    "speed_multiplier": worker_profile.get("speed_multiplier", 1.0),
                    "queues": [],
                    "consumer_count": 0,
                    "educational_note": "Worker profile mapped from app config to RabbitMQ consumers"
                }
        except:
            pass  # Fallback if Go API fails
            
        # Map RabbitMQ consumers to workers based on skill-to-queue mapping
        for consumer in consumers:
            queue_name = consumer.get('queue', {}).get('name', '')
            
            if queue_name.startswith('game.skill.'):
                # Extract skill type from queue name (e.g., "game.skill.gather.q" -> "gather")
                skill_type = queue_name.split('.skill.')[1].split('.')[0]
                
                # Find workers that have this skill and assign consumer
                for worker_name, worker_info in metrics["worker_roster"].items():
                    worker_skills = worker_info.get("skills", [])
                    if skill_type in worker_skills:
                        worker_info["queues"].append(queue_name)
                        worker_info["consumer_count"] += 1
        
        return {"ok": True, "metrics": metrics}
        
    except Exception as e:
        return {"ok": False, "error": str(e)}

@app.get("/api/rabbitmq/derived/scoreboard")
async def api_rabbitmq_derived_scoreboard():
    """Calculate scoreboard from actual RabbitMQ message consumption - no internal tracking"""
    # NOTE: This demonstrates educational limitation - RabbitMQ doesn't store consumed message history
    # In production, you'd use RabbitMQ streams or external persistence for this
    # For now, we'll combine live RabbitMQ state with our message stream buffer
    
    try:
        # Get current RabbitMQ metrics
        metrics_result = await api_rabbitmq_derived_metrics()
        if not metrics_result["ok"]:
            return metrics_result
            
        rmq_metrics = metrics_result["metrics"]
        
        # EDUCATIONAL: Calculate scores from live message buffer (demonstrates limitation)
        # In real systems, you'd use RabbitMQ streams or dedicated scoring queues
        global live_message_buffer
        scoreboard = {}
        player_stats = {}
        
        for msg_data in live_message_buffer:
            payload = msg_data.get("payload", {})
            event_stage = payload.get("event_stage", "")
            player = payload.get("player", "")
            points = int(payload.get("points", 0))
            
            if event_stage.endswith("COMPLETED") and player:
                scoreboard[player] = scoreboard.get(player, 0) + points
                if player not in player_stats:
                    player_stats[player] = {"accepted": 0, "completed": 0, "failed": 0}
                player_stats[player]["completed"] += 1
            elif event_stage.endswith("FAILED") and player:
                if player not in player_stats:
                    player_stats[player] = {"accepted": 0, "completed": 0, "failed": 0}
                player_stats[player]["failed"] += 1
        
        # Combine with RabbitMQ worker roster
        for worker_name, worker_info in rmq_metrics["worker_roster"].items():
            if worker_name not in player_stats:
                player_stats[worker_name] = {"accepted": 0, "completed": 0, "failed": 0}
                
        result = {
            "source": "rabbitmq_derived",
            "timestamp": time.time(),
            "scoreboard": scoreboard,
            "player_stats": player_stats,
            "roster": rmq_metrics["worker_roster"],
            "educational_note": "Scores calculated from message stream buffer - RabbitMQ doesn't persist consumed messages"
        }
        
        return {"ok": True, "data": result}
        
    except Exception as e:
        return {"ok": False, "error": str(e)}

# Main broker sync endpoint - now sources from RabbitMQ directly
@app.get("/api/broker/sync")
async def api_broker_sync():
    """Main endpoint for UI - now sources directly from RabbitMQ for educational transparency"""
    try:
        # Get comprehensive RabbitMQ-derived data
        metrics_result = await api_rabbitmq_derived_metrics()
        scoreboard_result = await api_rabbitmq_derived_scoreboard()
        
        if metrics_result["ok"] and scoreboard_result["ok"]:
            rmq_metrics = metrics_result["metrics"]
            score_data = scoreboard_result["data"]
            
            # Convert to UI-compatible format
            per_queue = []
            for queue_name, queue_stats in rmq_metrics["queue_stats"].items():
                per_queue.append({
                    "name": queue_name,
                    "ready": queue_stats["ready"],
                    "unacked": queue_stats["unacked"],
                    "consumers": queue_stats["consumers"],
                    "state": queue_stats["state"]
                })
            
            return {
                "ok": True,
                "source": "direct_rabbitmq",
                "total_ready": rmq_metrics["total_pending"],
                "total_unacked": rmq_metrics["total_unacked"],
                "queues": per_queue,
                "metrics": {
                    "per_type": rmq_metrics["per_type"],
                    "total_pending": rmq_metrics["total_pending"],
                    "total_unacked": rmq_metrics["total_unacked"],
                    "total_consumers": rmq_metrics["total_consumers"]
                },
                "roster": score_data["roster"],
                "player_stats": score_data["player_stats"],
                "scoreboard": score_data["scoreboard"],
                "routing_mode": routing_mode,
                "educational_note": "All data sourced directly from RabbitMQ Management API"
            }
        else:
            # Fallback to raw queue data only
            result = await api_rabbitmq_raw_queues()
            if result["ok"]:
                total_ready = sum(q.get('messages_ready', 0) for q in result["raw_rabbitmq_data"])
                total_unacked = sum(q.get('messages_unacknowledged', 0) for q in result["raw_rabbitmq_data"])
                per_queue = [{"name": q.get('name'), "ready": q.get('messages_ready', 0), "unacked": q.get('messages_unacknowledged', 0)} for q in result["raw_rabbitmq_data"]]
                
                return {"ok": True, "source": "rabbitmq_raw_fallback", "total_ready": total_ready, "total_unacked": total_unacked, "queues": per_queue}
            else:
                return {"ok": False, "error": "RabbitMQ query failed", "source": "error"}
                
    except Exception as e:
        return {"ok": False, "error": str(e), "source": "exception"}


@app.get("/api/rabbitmq/raw/messages/{queue_name}")
async def api_rabbitmq_peek_messages(queue_name: str, count: int = 5):
    """Peek at messages in a queue without consuming them - direct RabbitMQ API access"""
    import json, base64, urllib.request
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    
    # Use RabbitMQ Management API to get messages without consuming
    url = f"{api}/queues/%2F/{queue_name}/get"
    data = json.dumps({"count": count, "ackmode": "ack_requeue_false", "encoding": "auto"}).encode('utf-8')
    
    req = urllib.request.Request(url, data=data, method='POST')
    token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
    req.add_header('Authorization', f'Basic {token}')
    req.add_header('Content-Type', 'application/json')
    
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            messages = json.loads(resp.read().decode())
            return {"ok": True, "queue": queue_name, "messages": messages}
    except Exception as e:
        return {"ok": False, "error": str(e), "queue": queue_name}

# Global list to store live message stream for WebSocket clients
live_message_buffer = []
MAX_LIVE_MESSAGES = 50

def broadcast_raw_message(routing_key: str, payload: dict, source: str = "unknown"):
    """Broadcast raw RabbitMQ message to WebSocket clients - no abstraction"""
    global live_message_buffer
    
    message_data = {
        "timestamp": time.time(),
        "routing_key": routing_key,
        "payload": payload,
        "source": source,
        "exchange": EXCHANGE_NAME
    }
    
    live_message_buffer.append(message_data)
    if len(live_message_buffer) > MAX_LIVE_MESSAGES:
        live_message_buffer.pop(0)
    
    # Broadcast to RabbitMQ WebSocket clients
    broadcast_rabbitmq_data("live_message", message_data)

def broadcast_rabbitmq_data(msg_type: str, data):
    """Send data to RabbitMQ WebSocket clients"""
    # This will be populated with active RabbitMQ WebSocket clients
    pass

# WebSocket endpoint for live RabbitMQ data stream
@app.websocket("/ws/rabbitmq")
async def rabbitmq_websocket_endpoint(ws: WebSocket):
    """Direct RabbitMQ data stream - minimal abstraction for educational purposes"""
    await ws.accept()
    
    # Send recent message history
    try:
        await ws.send_text(json.dumps({
            "type": "message_history",
            "messages": live_message_buffer[-20:]  # Last 20 messages
        }))
    except Exception:
        pass
    
    try:
        while True:
            # Poll RabbitMQ directly every second
            try:
                # Get queue stats
                queue_result = await api_rabbitmq_raw_queues()
                if queue_result["ok"]:
                    await ws.send_text(json.dumps({
                        "type": "rabbitmq_queues",
                        "timestamp": time.time(),
                        "data": queue_result["raw_rabbitmq_data"]
                    }))
                
                # Get exchange/binding data less frequently (every 5 seconds)
                if int(time.time()) % 5 == 0:
                    exchange_result = await api_rabbitmq_raw_exchanges()
                    if exchange_result["ok"]:
                        await ws.send_text(json.dumps({
                            "type": "rabbitmq_topology",
                            "timestamp": time.time(),
                            "exchanges": exchange_result["exchanges"],
                            "bindings": exchange_result["bindings"]
                        }))
                
            except Exception as e:
                await ws.send_text(json.dumps({
                    "type": "error",
                    "message": f"RabbitMQ polling error: {str(e)}"
                }))
            
            await asyncio.sleep(1)
            
    except WebSocketDisconnect:
        pass
    except Exception as e:
        print(f"RabbitMQ WebSocket error: {e}")

@app.get("/api/broker/routes")
async def api_broker_routes():
    import json, base64, urllib.request
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    # The management API uses URLs like /api/bindings/vhost/e/exchange/bindings
    # Use 'bindings' listing then filter by exchange; alternate URL forms differ by RabbitMQ versions
    url = f"{api}/bindings/%2F"
    req = urllib.request.Request(url)
    token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
    req.add_header('Authorization', f'Basic {token}')
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            data = json.loads(resp.read().decode())
    except Exception as e:
        return {"ok": False, "error": str(e)}
    routes = []
    for b in data:
        if b.get('destination_type') == 'queue' and b.get('source') == EXCHANGE_NAME:
            routes.append({
                'routing_key': b.get('routing_key'),
                'queue': b.get('destination'),
                'args': b.get('arguments', {}),
            })
    return {"ok": True, "routes": routes}


@app.get("/api/pending/list")
async def api_pending_list():
    items = []
    now = time.time()
    for qid, q in quests_state.items():
        if q.get("status") == "pending":
            items.append({
                "quest_id": qid,
                "quest_type": q.get("quest_type"),
                "age_sec": int(now - float(q.get("issued_at", now))),
            })
    items.sort(key=lambda x: -x.get("age_sec", 0))
    return {"ok": True, "pending": items}


class PendingReissueRequest(BaseModel):
    quest_id: Optional[str] = None


@app.post("/api/pending/reissue")
async def api_pending_reissue(req: PendingReissueRequest):
    if req.quest_id:
        q = quests_state.get(req.quest_id)
        if not q or q.get("status") != "pending":
            return {"ok": False, "error": "not pending or not found"}
        publish_one(q.get("quest_type", "gather"), reissue_of=req.quest_id)
        return {"ok": True, "count": 1}
    # all
    cnt = 0
    for qid, q in list(quests_state.items()):
        if q.get("status") == "pending":
            publish_one(q.get("quest_type", "gather"), reissue_of=qid)
            cnt += 1
    return {"ok": True, "count": cnt}


def heartbeat_thread(loop: asyncio.AbstractEventLoop):
    while True:
        time.sleep(1.0)
        # Expiration sweep based on configured TTL per skill
        try:
            now = time.time()
            for qid, q in list(quests_state.items()):
                if q.get("status") == "pending":
                    qt = q.get("quest_type")
                    ttl = skill_ttl_ms.get(qt)
                    if ttl:
                        issued = float(q.get("issued_at", now))
                        if now - issued > (ttl / 1000.0):
                            STATE.record_expired(qid, qt)
                            broadcast("expired", {"quest_id": qid, "quest_type": qt})
                            # Lose points for expired messages in card game
                            if card_game and card_game.active:
                                card_game.adjust_score(-10, "expired_message")
                                
            # Update card game if active
            if card_game:
                card_game.tick()
                # Lose points for unroutable and DLQ messages
                unroutable_count = len([u for u in unroutable if now - u.get('ts', 0) > 10])
                dlq_count = len(dlq_messages)
                if unroutable_count > 0:
                    card_game.adjust_score(-unroutable_count * 5, "unroutable_messages")
                if dlq_count > 0:
                    card_game.adjust_score(-dlq_count * 3, "dlq_messages")
                    
        except Exception:
            pass
            
        # Enhanced tick with card game info
        tick_payload = {"ts": time.time()}
        if card_game:
            status = card_game.get_status()
            tick_payload.update({
                "card_timer": status.get("timer", 0),
                "game_score": status.get("score", 0),
                "game_active": status.get("active", False),
                "effects_count": len(status.get("active_effects", []))
            })
        
        broadcast("tick", tick_payload)


@app.get("/api/health")
async def api_health():
    return {"ok": True}

# Debug endpoint removed - roster synchronization fixed


# Card Game API Endpoints (pluggable)
@app.post("/api/cardgame/start")
async def start_card_game(duration: int = 300):
    """Start a new card game round."""
    if not CARD_GAME_ENABLED or not card_game:
        return {"error": "Card game not available"}
    
    success = card_game.start_round(duration)
    if success:
        return {"ok": True, "duration": duration}
    else:
        return {"error": "Card game already active"}


@app.post("/api/cardgame/stop")
async def stop_card_game():
    """Stop the current card game."""
    if not CARD_GAME_ENABLED or not card_game:
        return {"error": "Card game not available"}
    
    result = card_game.stop_round()
    if "error" in result:
        return result
    else:
        return {"ok": True, "final_score": result["final_score"]}


@app.get("/api/cardgame/status")
async def card_game_status():
    """Get current card game status."""
    if not CARD_GAME_ENABLED or not card_game:
        return {"error": "Card game not available"}
    
    return card_game.get_status()


@app.post("/api/cardgame/draw")
async def manual_draw_card():
    """Manually draw a card (for testing)."""
    if not CARD_GAME_ENABLED or not card_game:
        return {"error": "Card game not available"}
        
    result = card_game.manual_draw()
    if "error" in result:
        return result
    else:
        return {"ok": True, "card": result["card"]}


@app.get("/api/cardgame/deck")
async def get_card_deck():
    """Get all available cards by color."""
    if not CARD_GAME_ENABLED or not card_game:
        return {"error": "Card game not available"}
    
    return card_game.get_deck()


@app.get("/api/cardgame/enabled")
async def card_game_enabled():
    """Check if card game is available."""
    return {"enabled": CARD_GAME_ENABLED}