from __future__ import annotations
import threading
from typing import Callable, Dict
from app.state import GameState
from app.bus import BusClient


def scenario_redelivery(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    players.setdefault("alice", {}).setdefault("controls", {})["next_action"] = "drop"
    threading.Thread(target=lambda: bus.publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()


def scenario_requeue(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    players.setdefault("alice", {}).setdefault("controls", {})["next_action"] = "requeue"
    threading.Thread(target=lambda: bus.publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()


def scenario_duplicate(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    threading.Thread(target=lambda: bus.publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()


def scenario_both_complete(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    threading.Thread(target=lambda: bus.publish_wave(1, 0.1, fixed_type="slay"), daemon=True).start()


def scenario_dlq_poison(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    players.setdefault("alice", {}).setdefault("controls", {})["next_action"] = "dlq"
    threading.Thread(target=lambda: bus.publish_wave(1, 0.1, fixed_type="gather"), daemon=True).start()


NAME_TO_SCENARIO: Dict[str, Callable[[GameState, BusClient, Callable, dict], None]] = {
    "redelivery": scenario_redelivery,
    "requeue": scenario_requeue,
    "duplicate": scenario_duplicate,
    "both_complete": scenario_both_complete,
    "dlq_poison": scenario_dlq_poison,
}