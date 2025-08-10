#!/usr/bin/env python
import json
from collections import defaultdict
from app.event_bus import RabbitEventBus, EXCHANGE_NAME

SCORES = defaultdict(int)
FAILS = defaultdict(int)
QUEUE = "game.scoreboard.q"

if __name__ == "__main__":
    bus = RabbitEventBus()

    # Receive both done and fail events
    bus.ch.queue_declare(queue=QUEUE, durable=False, auto_delete=True)
    bus.ch.queue_bind(queue=QUEUE, exchange=EXCHANGE_NAME, routing_key="game.quest.*.done")
    bus.ch.queue_bind(queue=QUEUE, exchange=EXCHANGE_NAME, routing_key="game.quest.*.fail")
    bus.ch.basic_qos(prefetch_count=50)

    print("[scoreboard] listening for quest results … Ctrl+C to stop")

    def on_msg(ch, method, props, body):
        evt = json.loads(body.decode("utf-8"))
        player = (evt.get("player") or "?")
        points = int(evt.get("points", 0))
        rk = method.routing_key
        if rk.endswith(".done"):
            SCORES[player] += points
        else:
            FAILS[player] += 1
        ch.basic_ack(delivery_tag=method.delivery_tag)

        # Print a compact scoreboard
        leaderboard = sorted(SCORES.items(), key=lambda kv: kv[1], reverse=True)
        summary = ", ".join([f"{p}:{s}pts" for p, s in leaderboard]) or "(no scores yet)"
        failures = ", ".join([f"{p}:{n}x" for p, n in FAILS.items() if n])
        if failures:
            summary += f" | fails: {failures}"
        print(f"[scoreboard] {summary}")

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