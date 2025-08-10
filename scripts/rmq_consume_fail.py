#!/usr/bin/env python
import json
from app.event_bus import RabbitEventBus, EXCHANGE_NAME

PROCESS_QUEUE = "demo.orders.fail.q"
DLX_EXCHANGE = "rte.dlx"
DLQ_ROUTING = "rte.retrieve.orders.dlq"
DLQ_QUEUE = "demo.orders.dlq.q"
ROUTING_KEY = "rte.retrieve.orders"

if __name__ == "__main__":
    bus = RabbitEventBus()

    # Declare a dead-letter exchange and DLQ
    bus.ch.exchange_declare(exchange=DLX_EXCHANGE, exchange_type="direct", durable=True)
    bus.ch.queue_declare(queue=DLQ_QUEUE, durable=False, auto_delete=True)
    bus.ch.queue_bind(queue=DLQ_QUEUE, exchange=DLX_EXCHANGE, routing_key=DLQ_ROUTING)

    # Declare processing queue with DLX configuration
    bus.ch.queue_declare(
        queue=PROCESS_QUEUE,
        durable=False,
        auto_delete=True,
        arguments={
            "x-dead-letter-exchange": DLX_EXCHANGE,
            "x-dead-letter-routing-key": DLQ_ROUTING,
        },
    )
    bus.ch.queue_bind(queue=PROCESS_QUEUE, exchange=EXCHANGE_NAME, routing_key=ROUTING_KEY)

    # Limit to one unacked at a time to make flow visible
    bus.ch.basic_qos(prefetch_count=1)

    def failing_cb(ch, method, props, body):
        payload = json.loads(body.decode("utf-8"))
        print("[fail-consumer] processing -> will NACK to DLQ:", payload)
        ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)

    def dlq_cb(ch, method, props, body):
        payload = json.loads(body.decode("utf-8"))
        print("[DLQ] got message:", payload)
        ch.basic_ack(delivery_tag=method.delivery_tag)

    print(f"[fail-consumer] consuming {PROCESS_QUEUE}; DLQ is {DLQ_QUEUE}. Ctrl+C to stop")
    try:
        bus.ch.basic_consume(queue=PROCESS_QUEUE, on_message_callback=failing_cb, auto_ack=False)
        bus.ch.basic_consume(queue=DLQ_QUEUE, on_message_callback=dlq_cb, auto_ack=False)
        bus.ch.start_consuming()
    except KeyboardInterrupt:
        try:
            bus.ch.stop_consuming()
        except Exception:
            pass
    finally:
        bus.close()