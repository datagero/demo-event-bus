from __future__ import annotations
import threading
import time
from typing import Callable, Dict
from app.state import GameState
from app.bus import BusClient


def _ensure_players(players):
    # Ensure baseline players exist with expected skills
    players.setdefault("alice", {}).setdefault("controls", {})
    players.setdefault("bob", {}).setdefault("controls", {})


def scenario_redelivery(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    _ensure_players(players)
    broadcast("scenario", {"name": "redelivery", "desc": "If a player disconnects before ack, the message returns to Ready and is redelivered."})
    players["alice"]["controls"]["next_action"] = "drop"
    threading.Thread(target=lambda: bus.publish_wave(1, 0.05, fixed_type="slay"), daemon=True).start()


def scenario_requeue(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    _ensure_players(players)
    broadcast("scenario", {"name": "requeue", "desc": "NACK with requeue=true returns the message to Ready immediately."})
    players["alice"]["controls"]["next_action"] = "requeue"
    threading.Thread(target=lambda: bus.publish_wave(1, 0.05, fixed_type="slay"), daemon=True).start()


def scenario_duplicate(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    _ensure_players(players)
    broadcast("scenario", {"name": "duplicate", "desc": "Player-based routing: same quest delivered to multiple player queues (each has their own copy)."})
    threading.Thread(target=lambda: bus.publish_wave(1, 0.05, fixed_type="slay"), daemon=True).start()


def scenario_both_complete(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    _ensure_players(players)
    broadcast("scenario", {"name": "both_complete", "desc": "Two players complete their own copies under player-based routing."})
    threading.Thread(target=lambda: bus.publish_wave(1, 0.05, fixed_type="slay"), daemon=True).start()


def scenario_dlq_poison(state: GameState, bus: BusClient, broadcast: Callable, players) -> None:
    _ensure_players(players)
    broadcast("scenario", {"name": "dlq_poison", "desc": "Poison message is NACKed to DLQ for triage."})
    players["alice"]["controls"]["next_action"] = "dlq"
    threading.Thread(target=lambda: bus.publish_wave(1, 0.05, fixed_type="gather"), daemon=True).start()


NAME_TO_SCENARIO: Dict[str, Callable[[GameState, BusClient, Callable, dict], None]] = {
    "redelivery": scenario_redelivery,
    "requeue": scenario_requeue,
    "duplicate": scenario_duplicate,
    "both_complete": scenario_both_complete,
    "dlq_poison": scenario_dlq_poison,
}