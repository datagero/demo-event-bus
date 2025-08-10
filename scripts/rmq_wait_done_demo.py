#!/usr/bin/env python
from app.event_bus import RabbitEventBus, build_message

if __name__ == "__main__":
    case_id = "demo-case-1"
    bus = RabbitEventBus()

    # Orchestrator-style temp queue: listen to all *.done
    q = "demo.wait.q"
    bus.declare_queue(q, "rte.retrieve.*.done")

    # Simulate the sidecar finishing:
    bus.publish("rte.retrieve.orders.done",
                build_message(case_id, "RETRIEVE_COMPLETED", "SUCCESS", "demo"))

    matched = bus.wait_for_messages(
        q,
        predicate=lambda p: p.get("case_id")==case_id and p.get("event_stage")=="RETRIEVE_COMPLETED",
        expected_count=1,
        timeout_sec=10
    )
    print("matched:", matched)
    bus.close()