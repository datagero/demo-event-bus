import asyncio
import json
import os
import threading
import time
from dataclasses import dataclass, field
from typing import Dict, List, Optional

from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import HTMLResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel
from app.state import GameState

from app.event_bus import RabbitEventBus, EXCHANGE_NAME, build_message
from app.bus import BusClient
from app import scenarios

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
# Global one-shot chaos action for the next message picked by any player
next_action_global: Optional[str] = None
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

ENABLE_SCOREBOARD_CONSUMER = os.getenv("ENABLE_SCOREBOARD_CONSUMER", "1") not in {"0", "false", "False"}
RETRY_SEC = float(os.getenv("RABBITMQ_RETRY_SEC", "2.0"))


def broadcast(type_: str, payload: dict):
    if broadcaster:
        snap_metrics = STATE.snapshot()
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
        }
        broadcaster.broadcast_threadsafe(snap)


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
    # Bind to issued quests and results
    bus.declare_queue(q, "game.quest.*")
    bus.ch.queue_bind(queue=q, exchange=EXCHANGE_NAME, routing_key="game.quest.*.done")
    bus.ch.queue_bind(queue=q, exchange=EXCHANGE_NAME, routing_key="game.quest.*.fail")

    def handler(payload: dict, ack):
        # Determine event type from payload
        event_stage = payload.get("event_stage", "")
        player = payload.get("player") or ""
        points = int(payload.get("points", 0))

        if event_stage.endswith("COMPLETED"):
            # dedupe by quest id
            cid = payload.get("case_id")
            if cid and cid not in processed_results and player:
                processed_results.add(cid)
                scoreboard[player] = scoreboard.get(player, 0) + points
                STATE.record_done(player, cid, payload.get("quest_type", "gather"))
            evt_type = "result_done"
        elif event_stage.endswith("FAILED"):
            cid = payload.get("case_id")
            if cid and cid not in processed_results and player:
                processed_results.add(cid)
                fails[player] = fails.get(player, 0) + 1
                STATE.record_fail(player, cid, payload.get("quest_type", "gather"))
            evt_type = "result_fail"
        elif event_stage == "QUEST_ISSUED":
            cid = payload.get("case_id")
            if cid:
                STATE.record_issued(cid, payload.get("quest_type", "gather"))
            evt_type = "quest_issued"
        else:
            evt_type = "event"

        broadcast(evt_type, payload)
        ack()

    try:
        bus.consume_forever(q, handler)
    finally:
        try:
            bus.close()
        except Exception:
            pass


def start_player_thread(player: str, skills: List[str], fail_pct: float, speed_multiplier: float = 1.0, workers: int = 1):
    def run():
        queue_name = f"game.player.{player}.q"
        # ensure registry and controls
        controls = players.setdefault(player, {}).setdefault("controls", {"paused": False, "next_action": None})
        while True:
            # connect with retry
            bus: RabbitEventBus | None = None
            while bus is None:
                try:
                    bus = RabbitEventBus()
                except Exception:
                    time.sleep(RETRY_SEC)

            # declarations
            try:
                pref = int(roster.get(player, {}).get("prefetch", 1))
                pref = max(1, min(100, pref))
                bus.ch.basic_qos(prefetch_count=pref)
                if routing_mode == "player":
                    # each player gets a copy; will skip non-skilled
                    bus.ch.queue_declare(queue=queue_name, durable=False, auto_delete=True)
                    bus.ch.queue_bind(queue=queue_name, exchange=EXCHANGE_NAME, routing_key="game.quest.*")
                else:
                    # shared per-skill queues; consume only skills
                    for sk in skills:
                        qn = f"game.skill.{sk}.q"
                        bus.ch.queue_declare(queue=qn, durable=False, auto_delete=True)
                        bus.ch.queue_bind(queue=qn, exchange=EXCHANGE_NAME, routing_key=f"game.quest.{sk}")
                # publish presence
                roster[player] = {**roster.get(player, {}), "status": "online"}
                players[player]["bus"] = bus
                broadcast("player_online", {"player": player})
            except Exception:
                try:
                    bus.close()
                except Exception:
                    pass
                time.sleep(RETRY_SEC)
                continue

            def on_msg(ch, method, props, body):
                payload = json.loads(body.decode("utf-8"))
                quest_type = payload.get("quest_type")
                difficulty = payload.get("difficulty", "medium")
                work_sec = float(payload.get("work_sec", 2.0))
                points = int(payload.get("points", 5))
                quest_id = payload.get("case_id")

                # pause handling: immediately requeue to avoid tight loops
                if controls.get("paused"):
                    ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)
                    broadcast("player_reconnecting", {"player": player})
                    time.sleep(0.2)
                    return

                import random as _rand
                # chaos skipping
                if (roster.get(player, {}).get("skip_rate", 0.0) or 0.0) > 0 and _rand.random() < float(roster[player].get("skip_rate", 0.0)):
                    key = (player, quest_id)
                    if key not in skip_logged:
                        skip_logged.add(key)
                        broadcast("player_skip", {"player": player, "quest_id": quest_id, "quest_type": quest_type})
                    ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)
                    return

                if routing_mode == "player" and quest_type not in skills:
                    # log skip once per (player, quest)
                    key = (player, quest_id)
                    if key not in skip_logged:
                        skip_logged.add(key)
                        broadcast("player_skip", {"player": player, "quest_id": quest_id, "quest_type": quest_type})
                    ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)
                    return

                broadcast("player_accept", {
                    "player": player,
                    "quest_id": quest_id,
                    "quest_type": quest_type,
                    "difficulty": difficulty,
                    "work_sec": work_sec * max(0.05, speed_multiplier),
                    "points": points,
                })
                STATE.record_accepted(
                    quest_id=quest_id,
                    quest_type=quest_type,
                    player=player,
                    work_sec=work_sec * max(0.05, speed_multiplier),
                )
                # remember current inflight so unexpected crash can be logged
                players.setdefault(player, {}).setdefault("runtime", {})["last_inflight"] = quest_id

                # one-shot next action simulation (player-specific or global)
                global next_action_global
                act = controls.get("next_action") or next_action_global
                if act:
                    # clear player-local and global arm after consuming once
                    controls["next_action"] = None
                    if next_action_global:
                        next_action_global = None
                    if act == "drop":
                        # don't ack, simulate crash so it is redelivered later
                        roster[player] = {**roster.get(player, {}), "status": "reconnecting"}
                        broadcast("player_reconnecting", {"player": player})
                        # tell UI which quest was dropped
                        broadcast("msg_drop", {"player": player, "quest_id": quest_id, "quest_type": quest_type})
                        STATE.record_drop(player, quest_id, quest_type)
                        try:
                            bus.conn.close()
                        except Exception:
                            pass
                        # set back to pending for KPI and UI
                        return
                    if act == "requeue":
                        broadcast("msg_requeue", {"player": player, "quest_id": quest_id, "quest_type": quest_type})
                        STATE.record_requeue(player, quest_id, quest_type)
                        try:
                            inflight_by_player.get(player, set()).discard(quest_id)
                        except Exception:
                            pass
                        # set back to pending for KPI and UI
                        return
                    if act == "dlq":
                        broadcast("msg_dlq", {"player": player, "quest_id": quest_id, "quest_type": quest_type})
                        STATE.record_dlq(player, quest_id, quest_type)
                        try:
                            inflight_by_player.get(player, set()).discard(quest_id)
                        except Exception:
                            pass
                        # record in in-memory DLQ
                        try:
                            dlq_messages.append({
                                "quest_id": quest_id,
                                "quest_type": quest_type,
                                "player": player,
                                "ts": time.time(),
                            })
                        except Exception:
                            pass
                        ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
                        return
                    if act == "fail_early":
                        done_stage = "QUEST_FAILED"; status = "FAILED"; rk_suffix = "fail"
                        result = build_message(
                            case_id=quest_id,
                            event_stage=done_stage,
                            status=status,
                            source=f"player:{player}",
                            extra={
                                "quest_type": quest_type,
                                "difficulty": difficulty,
                                "points": points,
                                "player": player,
                            },
                        )
                        try:
                            bus.publish(f"game.quest.{quest_type}.{rk_suffix}", result)
                        except Exception:
                            pass
                        # update scoreboard locally as well
                        fails[player] = fails.get(player, 0) + 1
                        broadcast("result_fail", result)
                        ch.basic_ack(delivery_tag=method.delivery_tag)
                        return

                # Work (scaled by speed multiplier; smaller is faster)
                time.sleep(work_sec * max(0.05, speed_multiplier))
                # chaos drop after work
                if (roster.get(player, {}).get("drop_rate", 0.0) or 0.0) > 0 and _rand.random() < float(roster[player].get("drop_rate", 0.0)):
                    STATE.record_drop(player, quest_id, quest_type)
                    broadcast("msg_drop", {"player": player, "quest_id": quest_id, "quest_type": quest_type})
                    try:
                        bus.conn.close()
                    except Exception:
                        pass
                    return
                # Determine outcome
                import random
                if random.random() < fail_pct:
                    done_stage = "QUEST_FAILED"
                    status = "FAILED"
                    rk_suffix = "fail"
                else:
                    done_stage = "QUEST_COMPLETED"
                    status = "SUCCESS"
                    rk_suffix = "done"

                result = build_message(
                    case_id=quest_id,
                    event_stage=done_stage,
                    status=status,
                    source=f"player:{player}",
                    extra={
                        "quest_type": quest_type,
                        "difficulty": difficulty,
                        "points": points,
                        "player": player,
                    },
                )
                try:
                    bus.publish(f"game.quest.{quest_type}.{rk_suffix}", result)
                except Exception:
                    pass
                # update scoreboard locally so UI reflects even without internal consumer
                cid = result.get("case_id")
                if cid and cid not in processed_results:
                    processed_results.add(cid)
                    if status == "SUCCESS":
                        scoreboard[player] = scoreboard.get(player, 0) + points
                        STATE.record_done(player, cid, quest_type)
                        broadcast("result_done", result)
                    else:
                        fails[player] = fails.get(player, 0) + 1
                        STATE.record_fail(player, cid, quest_type)
                        broadcast("result_fail", result)
                    try:
                        inflight_by_player.get(player, set()).discard(cid)
                    except Exception:
                        pass
                ch.basic_ack(delivery_tag=method.delivery_tag)

            try:
                if routing_mode == "player":
                    bus.ch.basic_consume(queue=queue_name, on_message_callback=on_msg, auto_ack=False)
                else:
                    # consume from each skill queue
                    for sk in skills:
                        qn = f"game.skill.{sk}.q"
                        bus.ch.basic_consume(queue=qn, on_message_callback=on_msg, auto_ack=False)
                bus.ch.start_consuming()
            except Exception:
                roster[player] = {**roster.get(player, {}), "status": "reconnecting"}
                players[player]["bus"] = None
                broadcast("player_reconnecting", {"player": player})
                # If there was an inflight quest, mark as dropped and pending again for accurate timeline/KPI
                try:
                    last_q = players.get(player, {}).get("runtime", {}).get("last_inflight")
                    if last_q:
                        qtype = quests_state.get(last_q, {}).get("quest_type", "gather")
                        broadcast("msg_drop", {"player": player, "quest_id": last_q, "quest_type": qtype})
                        STATE.record_drop(player, last_q, qtype)
                        players[player]["runtime"]["last_inflight"] = None
                except Exception:
                    pass
                try:
                    bus.close()
                except Exception:
                    pass
                time.sleep(RETRY_SEC)
                continue

    t = threading.Thread(target=run, daemon=True)
    # Launch multiple worker threads (each holds its own connection/channel)
    t.start()
    for _ in range(max(0, workers - 1)):
        threading.Thread(target=run, daemon=True).start()


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


def publish_one(quest_type: str):
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
    quest_id = f"q-{int(_time.time())}-{random.randint(100,999)}"
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
    bus.publish(f"game.quest.{quest_type}", payload)
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
        controls["paused"] = True
        roster[req.player] = {**roster.get(req.player, {}), "status": "reconnecting"}
        broadcast("roster", {})
    elif req.action == "resume":
        controls["paused"] = False
        roster[req.player] = {**roster.get(req.player, {}), "status": "online"}
        broadcast("roster", {})
    elif req.action == "crash":
        bus = players[req.player].get("bus")
        try:
            if bus:
                bus.conn.close()
        except Exception:
            pass
        roster[req.player] = {**roster.get(req.player, {}), "status": "reconnecting"}
        broadcast("roster", {})
    elif req.action == "next_action":
        if req.mode in {"drop", "requeue", "dlq", "fail_early"}:
            controls["next_action"] = req.mode
        else:
            return {"ok": False, "error": "invalid mode"}
    else:
        return {"ok": False, "error": "invalid action"}
    return {"ok": True}


class RoutingModeRequest(BaseModel):
    mode: str  # skill|player


@app.post("/api/routing/set")
async def api_routing_set(req: RoutingModeRequest):
    global routing_mode
    if req.mode not in {"skill", "player"}:
        return {"ok": False, "error": "invalid mode"}
    routing_mode = req.mode
    broadcast("routing_mode", {"mode": routing_mode})
    return {"ok": True, "mode": routing_mode}


class ScenarioRequest(BaseModel):
    name: str  # redelivery|requeue


@app.post("/api/scenario/run")
async def api_scenario_run(req: ScenarioRequest):
    # Ensure baseline players
    if "alice" not in roster:
        start_player_thread("alice", ["gather", "slay"], 0.2, 1.0, 1)
        roster["alice"] = {"skills": ["gather","slay"], "fail_pct": 0.2, "speed_multiplier": 1.0, "workers": 1}
    if "bob" not in roster:
        start_player_thread("bob", ["slay", "escort"], 0.1, 0.7, 2)
        roster["bob"] = {"skills": ["slay","escort"], "fail_pct": 0.1, "speed_multiplier": 0.7, "workers": 2}
    broadcast("roster", {})

    if req.name in scenarios.NAME_TO_SCENARIO:
        threading.Thread(
            target=lambda: scenarios.NAME_TO_SCENARIO[req.name](STATE, BusClient(), broadcast, players),
            daemon=True,
        ).start()
        return {"ok": True}
    if req.name == "duplicate":
        # Switch to player-based so both players get a copy
        global routing_mode
        routing_mode = "player"
        broadcast("routing_mode", {"mode": routing_mode})
        threading.Thread(target=lambda: BusClient().publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()
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
        threading.Thread(target=lambda: BusClient().publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()
        return {"ok": True}
    if req.name == "dlq_poison":
        # Back-compat passthrough; scenarios handler also handles this
        players.setdefault("alice", {}).setdefault("controls", {})["next_action"] = "dlq"
        threading.Thread(target=lambda: BusClient().publish_wave(1, 0.1, fixed_type="gather"), daemon=True).start()
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
        publish_one(q.get("quest_type", "gather"))
        return {"ok": True, "count": 1}
    # retry all
    count = 0
    for qid, q in list(quests_state.items()):
        if q.get("status") == "failed":
            publish_one(q.get("quest_type", "gather"))
            count += 1
    return {"ok": True, "count": count}


@app.get("/api/dlq/list")
async def api_dlq_list():
    return {"ok": True, "items": dlq_messages}


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
                publish_one(item.get("quest_type", "gather"))
                requeued += 1
            else:
                remaining.append(item)
        dlq_messages = remaining
        broadcast("dlq_updated", {"count": len(dlq_messages)})
        return {"ok": True, "count": requeued}
    # requeue all
    for item in dlq_messages:
        publish_one(item.get("quest_type", "gather"))
    cnt = len(dlq_messages)
    dlq_messages = []
    broadcast("dlq_updated", {"count": 0})
    return {"ok": True, "count": cnt}


@app.post("/api/dlq/purge")
async def api_dlq_purge():
    dlq_messages.clear()
    broadcast("dlq_updated", {"count": 0})
    return {"ok": True}


@app.post("/api/reset")
async def api_reset():
    # Clear transient game state (players keep running)
    scoreboard.clear()
    fails.clear()
    quests_state.clear()
    player_stats.clear()
    processed_results.clear()
    skip_logged.clear()
    inflight_by_player.clear()
    dlq_messages.clear()
    broadcast("reset", {})
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
    mode: str  # drop|requeue|dlq|fail_early


@app.post("/api/chaos/arm")
async def api_chaos_arm(req: ChaosArmRequest):
    global next_action_global
    if req.mode not in {"drop", "requeue", "dlq", "fail_early"}:
        return {"ok": False, "error": "invalid mode"}
    next_action_global = req.mode
    return {"ok": True, "mode": next_action_global}


# Broker-backed KPI endpoint
@app.get("/api/broker/sync")
async def api_broker_sync():
    import json, base64, urllib.request
    api = os.getenv("RABBITMQ_API_URL", "http://localhost:15672/api")
    user = os.getenv("RABBITMQ_USER", "guest"); pwd = os.getenv("RABBITMQ_PASS", "guest")
    url = f"{api}/queues"
    req = urllib.request.Request(url)
    token = base64.b64encode(f"{user}:{pwd}".encode()).decode()
    req.add_header('Authorization', f'Basic {token}')
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            data = json.loads(resp.read().decode())
    except Exception as e:
        return {"ok": False, "error": str(e)}
    # Aggregate Ready/Unacked for our queues
    total_ready = 0
    total_unacked = 0
    per_queue = []
    for q in data:
        name = q.get('name', '')
        if name.startswith('game.') or name.startswith('web.'):
            r = int(q.get('messages_ready', 0)); u = int(q.get('messages_unacknowledged', 0))
            total_ready += r; total_unacked += u
            per_queue.append({"name": name, "ready": r, "unacked": u})
    return {"ok": True, "total_ready": total_ready, "total_unacked": total_unacked, "queues": per_queue}


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
        publish_one(q.get("quest_type", "gather"))
        return {"ok": True, "count": 1}
    # all
    cnt = 0
    for qid, q in list(quests_state.items()):
        if q.get("status") == "pending":
            publish_one(q.get("quest_type", "gather"))
            cnt += 1
    return {"ok": True, "count": cnt}


def heartbeat_thread(loop: asyncio.AbstractEventLoop):
    while True:
        time.sleep(1.0)
        broadcast("tick", {"ts": time.time()})


@app.get("/api/health")
async def api_health():
    return {"ok": True}