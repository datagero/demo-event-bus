import json, os, threading, time
import pika

EXCHANGE_NAME = os.getenv("RTE_EXCHANGE", "rte.topic")
EXCHANGE_TYPE = "topic"
RABBIT_URL = os.getenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/%2F")

def build_message(case_id, event_stage, status, source, extra=None):
    msg = {
        "case_id": case_id,
        "event_stage": event_stage,
        "status": status,
        "source": source,
        "ts": int(time.time()),
    }
    if extra:
        msg.update(extra)
    return msg

class RabbitEventBus:
    def __init__(self):
        params = pika.URLParameters(RABBIT_URL)
        self.conn = pika.BlockingConnection(params)
        self.ch = self.conn.channel()
        # durable exchange so it sticks around for demos; queues can be non-durable
        self.ch.exchange_declare(exchange=EXCHANGE_NAME, exchange_type=EXCHANGE_TYPE, durable=True)

    def publish(self, routing_key, payload: dict):
        body = json.dumps(payload).encode("utf-8")
        self.ch.basic_publish(
            exchange=EXCHANGE_NAME,
            routing_key=routing_key,
            body=body,
            properties=pika.BasicProperties(content_type="application/json")
        )

    def declare_queue(self, queue_name, routing_key):
        # non-durable, auto-delete queue for easy demos
        self.ch.queue_declare(queue=queue_name, durable=False, auto_delete=True)
        self.ch.queue_bind(queue=queue_name, exchange=EXCHANGE_NAME, routing_key=routing_key)

    def consume_forever(self, queue_name, handler):
        def _on_msg(ch, method, props, body):
            payload = json.loads(body.decode("utf-8"))
            def ack():
                ch.basic_ack(delivery_tag=method.delivery_tag)
            handler(payload, ack)

        self.ch.basic_qos(prefetch_count=10)
        self.ch.basic_consume(queue=queue_name, on_message_callback=_on_msg, auto_ack=False)
        try:
            print(f"[consume] waiting on {queue_name}… Ctrl+C to stop")
            self.ch.start_consuming()
        except KeyboardInterrupt:
            self.ch.stop_consuming()

    def wait_for_messages(self, queue_name, predicate, expected_count=1, timeout_sec=10):
        """
        Pull-style wait: returns list of matched payloads (length == expected_count) or [] on timeout.
        """
        matched = []
        end = time.time() + timeout_sec

        while time.time() < end and len(matched) < expected_count:
            method_frame, props, body = self.ch.basic_get(queue=queue_name, auto_ack=False)
            if not method_frame:
                time.sleep(0.1)
                continue

            payload = json.loads(body.decode("utf-8"))
            if predicate(payload):
                matched.append(payload)
            # ack regardless; this is a simple demo
            self.ch.basic_ack(delivery_tag=method_frame.delivery_tag)

        return matched

    def close(self):
        try:
            self.ch.close()
        except Exception:
            pass
        try:
            self.conn.close()
        except Exception:
            pass