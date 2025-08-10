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
        try:
            self.ch.exchange_declare(exchange=EXCHANGE_NAME, exchange_type=EXCHANGE_TYPE, durable=True)
        except Exception:
            pass
        self._return_callbacks = []
        try:
            self.ch.add_on_return_callback(self._on_return)
        except Exception:
            pass

    def _on_return(self, ch, method, properties, body):
        try:
            p = json.loads(body.decode("utf-8"))
        except Exception:
            p = {"raw": body.decode("utf-8", errors="ignore")}
        info = {
            "payload": p,
            "routing_key": getattr(method, "routing_key", None),
            "exchange": getattr(method, "exchange", EXCHANGE_NAME),
            "reply_code": getattr(method, "reply_code", None),
            "reply_text": getattr(method, "reply_text", None),
        }
        for cb in list(self._return_callbacks):
            try:
                cb(info)
            except Exception:
                pass

    def publish(self, routing_key, payload: dict, on_unroutable=None):
        body = json.dumps(payload).encode("utf-8")
        if on_unroutable:
            self._return_callbacks.append(on_unroutable)
        try:
            self.ch.basic_publish(
                exchange=EXCHANGE_NAME,
                routing_key=routing_key,
                body=body,
                mandatory=True,
                properties=pika.BasicProperties(
                    content_type="application/json",
                    delivery_mode=2  # Make message persistent
                )
            )
            # For BlockingConnection, returned messages are delivered via process_data_events
            if on_unroutable:
                try:
                    # give the broker time to deliver basic.return
                    self.conn.process_data_events(time_limit=1.0)
                except Exception:
                    pass
        finally:
            if on_unroutable and on_unroutable in self._return_callbacks:
                try:
                    self._return_callbacks.remove(on_unroutable)
                except ValueError:
                    pass

    def declare_queue(self, queue_name, routing_key):
        # durable queue for message persistence
        self.ch.queue_declare(queue=queue_name, durable=True, auto_delete=False)
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