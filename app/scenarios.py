from __future__ import annotations
import threading
import time
from typing import Callable, Dict
from app.state import GameState
# BusClient removed - scenarios now use Go workers
from app.rabbitmq_utils import build_message, SimpleRabbitMQ
import random

def publish_one(quest_type: str) -> dict:
    """Publish a single quest - simplified version for scenarios."""
    points_by_type = {"gather": 5, "slay": 10, "escort": 15}
    difficulty_choices = [("easy", 1.0), ("medium", 2.0), ("hard", 3.5)]
    difficulty, work_sec = random.choice(difficulty_choices)
    points = points_by_type.get(quest_type, 5)
    quest_id = f"q-{int(time.time())}-{random.randint(100,999)}"
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
    bus = SimpleRabbitMQ()
    bus.publish(f"game.quest.{quest_type}", payload)
    bus.close()
    return payload

def scenario_late_bind_escort(state: GameState, go_workers_client, broadcast: Callable, players) -> None:
    """
    Late-bind escort (backlog handoff) scenario.
    
    This demonstrates a unique RabbitMQ behavior that can't be replicated with simple chaos:
    1. Messages published before any queue exists are unroutable
    2. When a worker creates the queue, it only gets NEW messages
    3. Previous messages remain unroutable unless explicitly reissued
    """
    broadcast("scenario", {
        "name": "late_bind_escort", 
        "desc": "Late-bind escort: backlog handoff demo - shows unroutable → queue creation → backlog handling"
    })
    
    def run_scenario():
        # Reset game state first
        state.reset_game()
        # Remove all current players
        for player in list(players.keys()):
            if player in players:
                try:
                    players[player]["controls"]["shutdown"] = True
                    del players[player]
                except Exception:
                    pass
        broadcast("reset", {"reason": "Late-bind scenario reset"})
        
        time.sleep(1)
        
        # 1. Publish messages before any escort queue exists (will be unroutable)
        broadcast("scenario_step", {"step": "1", "desc": "Publishing escort messages before any queue exists..."})
        for i in range(3):
            publish_one("escort")
            time.sleep(0.1)
        
        time.sleep(2)
        
        # 2. Add first escort worker (tempb) - creates queue and bindings
        broadcast("scenario_step", {"step": "2", "desc": "Worker 'tempb' arrives and creates escort queue..."})
        # This will be handled by the web server's start_player endpoint
        
        time.sleep(2)
        
        # 3. Publish more messages (these WILL be queued)
        broadcast("scenario_step", {"step": "3", "desc": "Publishing new escort messages (these will be queued)..."})
        for i in range(2):
            publish_one("escort")
            time.sleep(0.1)
        
        time.sleep(2)
        
        # 4. Pause first worker
        broadcast("scenario_step", {"step": "4", "desc": "Worker 'tempb' goes on break (paused)..."})
        if "tempb" in players:
            players["tempb"]["controls"]["paused"] = True
        
        time.sleep(1)
        
        # 5. Publish more messages (pile up as Ready)
        broadcast("scenario_step", {"step": "5", "desc": "More messages arrive while worker is paused..."})
        for i in range(2):
            publish_one("escort")
            time.sleep(0.1)
        
        time.sleep(2)
        
        # 6. Add second worker (tempd) - will consume from backlog
        broadcast("scenario_step", {"step": "6", "desc": "Worker 'tempd' arrives and starts clearing backlog..."})
        # This will be handled by the web server's start_player endpoint
        
        broadcast("scenario_complete", {"name": "late_bind_escort"})
    
    threading.Thread(target=run_scenario, daemon=True).start()


def scenario_routing_comparison(state: GameState, go_workers_client, broadcast: Callable, players) -> None:
    """
    Routing mode comparison scenario.
    
    Shows the difference between skill-based and player-based routing
    with the same message being handled differently.
    """
    broadcast("scenario", {
        "name": "routing_comparison", 
        "desc": "Routing comparison: skill-based vs player-based message delivery patterns"
    })
    
    def run_scenario():
        broadcast("scenario_step", {"step": "1", "desc": "Publishing slay message - observe routing behavior..."})
        bus.publish_one("slay")
        
        time.sleep(3)
        
        broadcast("scenario_step", {"step": "2", "desc": "Switch routing mode to see different behavior..."})
        broadcast("scenario_complete", {"name": "routing_comparison"})
    
    threading.Thread(target=run_scenario, daemon=True).start()


NAME_TO_SCENARIO: Dict[str, Callable[[GameState, object, Callable, dict], None]] = {
    "late_bind_escort": scenario_late_bind_escort,
    "routing_comparison": scenario_routing_comparison,
}