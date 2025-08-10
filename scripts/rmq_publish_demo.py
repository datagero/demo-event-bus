#!/usr/bin/env python
from app.event_bus import RabbitEventBus, build_message

if __name__ == "__main__":
    bus = RabbitEventBus()
    case_id = "demo-case-1"
    payload = build_message(
        case_id,
        "RETRIEVE_FAN_OUT_STARTED",
        "IN_PROGRESS",
        "demo",
        {"refs": [{"key_field":"customer_email","key_value":"alice@example.com"}]}
    )
    bus.publish("rte.retrieve.orders", payload)
    print("[publisher] sent to rte.retrieve.orders")
    bus.close()