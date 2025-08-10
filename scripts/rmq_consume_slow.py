#!/usr/bin/env python
import json
import time
from app.event_bus import RabbitEventBus, EXCHANGE_NAME

QUEUE = "demo.orders.slow.q"
ROUTING_KEY = "rte.retrieve.orders"

if __name__ == "__main__":
    bus = RabbitEventBus()

    # Declare a separate queue for slow processing demo
    bus.ch.queue_declare(queue=QUEUE, durable=False, auto_delete=True)
    bus.ch.queue_bind(queue=QUEUE, exchange=EXCHANGE_NAME, routing_key=ROUTING_KEY)

    # Process one message at a time to visualize Unacked
    bus.ch.basic_qos(prefetch_count=1)

    def on_msg(ch, method, props, body):
        payload = json.loads(body.decode("utf-8"))
        print("[slow-consumer] received:", payload)
        time.sleep(5)  # simulate long processing
        ch.basic_ack(delivery_tag=method.delivery_tag)
        print("[slow-consumer] acked:", payload.get("case_id"))

    print(f"[slow-consumer] waiting on {QUEUE} … Ctrl+C to stop")
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