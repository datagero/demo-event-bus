import time
import random
from typing import Optional
from app.event_bus import RabbitEventBus, build_message


class BusClient:
    def __init__(self) -> None:
        pass

    def publish_one(self, quest_type: str) -> dict:
        """Publish a single quest event and return the payload (includes case_id)."""
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
        bus = RabbitEventBus()
        bus.publish(f"game.quest.{quest_type}", payload)
        try:
            bus.close()
        except Exception:
            pass
        return payload

    def publish_wave(self, count: int, delay: float, fixed_type: Optional[str] = None) -> list[dict]:
        """Publish a wave of quests; returns list of payloads in order published."""
        out: list[dict] = []
        for _ in range(count):
            qt = fixed_type or random.choice(["gather", "slay", "escort"])  # nosec - demo
            payload = self.publish_one(qt)
            out.append(payload)
            time.sleep(delay)
        return out