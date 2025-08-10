#!/usr/bin/env python
import os
import random
import time
from app.event_bus import RabbitEventBus, build_message

QUEST_TYPES = ["gather", "slay", "escort"]
POINTS_BY_TYPE = {"gather": 5, "slay": 10, "escort": 15}
DIFFICULTY_CHOICES = [
    ("easy", 1.0),
    ("medium", 2.0),
    ("hard", 3.5),
]

COUNT = int(os.getenv("COUNT", "12"))
DELAY_BETWEEN_SEC = float(os.getenv("DELAY", "0.2"))
ROUTING_PREFIX = "game.quest"

if __name__ == "__main__":
    random.seed()
    bus = RabbitEventBus()

    for i in range(COUNT):
        quest_type = random.choice(QUEST_TYPES)
        difficulty, work_sec = random.choice(DIFFICULTY_CHOICES)
        points = POINTS_BY_TYPE[quest_type]
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
            },
        )
        routing_key = f"{ROUTING_PREFIX}.{quest_type}"
        bus.publish(routing_key, payload)
        print(f"[game-master] issued {quest_type} ({difficulty}, {points} pts) -> {routing_key}")
        time.sleep(DELAY_BETWEEN_SEC)

    print("[game-master] done")
    bus.close()