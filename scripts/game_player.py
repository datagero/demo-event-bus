#!/usr/bin/env python
import json
import os
import random
import time
from app.event_bus import RabbitEventBus, EXCHANGE_NAME, build_message

PLAYER = os.getenv("PLAYER", "anon")
SKILLS = [s.strip() for s in os.getenv("SKILLS", "gather,slay,escort").split(",") if s.strip()]
FAIL_PCT = float(os.getenv("FAIL_PCT", "0.2"))
ROUTING_PREFIX = "game.quest"
QUEUE = f"game.player.{PLAYER}.q"

if __name__ == "__main__":
    random.seed()
    bus = RabbitEventBus()

    # Per-player queue, receive all quests and decide to accept or requeue
    bus.ch.queue_declare(queue=QUEUE, durable=False, auto_delete=True)
    bus.ch.queue_bind(queue=QUEUE, exchange=EXCHANGE_NAME, routing_key=f"{ROUTING_PREFIX}.*")
    bus.ch.basic_qos(prefetch_count=1)

    print(f"[player:{PLAYER}] skills={SKILLS}, fail%={int(FAIL_PCT*100)}; listening on {QUEUE}")

    def on_msg(ch, method, props, body):
        payload = json.loads(body.decode("utf-8"))
        quest_type = payload.get("quest_type") or payload.get("event_stage")
        difficulty = payload.get("difficulty", "medium")
        work_sec = float(payload.get("work_sec", 2.0))
        points = int(payload.get("points", 5))
        quest_id = payload.get("case_id")

        if quest_type not in SKILLS:
            print(f"[player:{PLAYER}] skipping quest {quest_id} type={quest_type} (not skilled)")
            ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)
            return

        print(f"[player:{PLAYER}] accepted {quest_id} {quest_type} ({difficulty}) -> working {work_sec}s")
        time.sleep(work_sec)

        if random.random() < FAIL_PCT:
            done_stage = "QUEST_FAILED"
            status = "FAILED"
            rk = f"{ROUTING_PREFIX}.{quest_type}.fail"
        else:
            done_stage = "QUEST_COMPLETED"
            status = "SUCCESS"
            rk = f"{ROUTING_PREFIX}.{quest_type}.done"

        result = build_message(
            case_id=quest_id,
            event_stage=done_stage,
            status=status,
            source=f"player:{PLAYER}",
            extra={
                "quest_type": quest_type,
                "difficulty": difficulty,
                "points": points,
                "player": PLAYER,
            },
        )
        bus.publish(rk, result)
        ch.basic_ack(delivery_tag=method.delivery_tag)
        print(f"[player:{PLAYER}] {status.lower()} {quest_id} -> {rk} (+{points} pts if success)")

    try:
        bus.ch.basic_consume(queue=QUEUE, on_message_callback=on_msg, auto_ack=False)
        bus.ch.start_consuming()
    except KeyboardInterrupt:
        try:
            bus.ch.stop_consuming()
        except Exception:
            pass
    finally:
        bus.close()