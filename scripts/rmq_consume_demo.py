#!/usr/bin/env python
from app.event_bus import RabbitEventBus

def handle(payload, ack):
    print("[consumer] got:", payload)
    ack()

if __name__ == "__main__":
    bus = RabbitEventBus()
    # Bind a demo queue to orders retrieve events
    bus.declare_queue("demo.orders.q", "rte.retrieve.orders")
    bus.consume_forever("demo.orders.q", handle)