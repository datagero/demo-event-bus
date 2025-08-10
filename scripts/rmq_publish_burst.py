#!/usr/bin/env python
import os
import time
from app.event_bus import RabbitEventBus, build_message

COUNT = int(os.getenv("COUNT", "10"))
SLEEP_SEC = float(os.getenv("SLEEP", "0.1"))
ROUTING_KEY = "rte.retrieve.orders"

if __name__ == "__main__":
    bus = RabbitEventBus()
    for i in range(COUNT):
        case_id = f"demo-burst-{int(time.time())}-{i}"
        payload = build_message(
            case_id,
            "RETRIEVE_FAN_OUT_STARTED",
            "IN_PROGRESS",
            "burst",
            {"i": i},
        )
        bus.publish(ROUTING_KEY, payload)
        print(f"[burst] sent {i+1}/{COUNT}")
        time.sleep(SLEEP_SEC)
    bus.close()