"""
Minimal RabbitMQ utilities for Go-only system.
This replaces the deleted event_bus.py with essential constants and functions.
"""
import os
import json
import time
import uuid
import pika

# RabbitMQ constants
EXCHANGE_NAME = os.getenv("RTE_EXCHANGE", "rte.topic")
EXCHANGE_TYPE = "topic"

def build_message(case_id, event_stage, status, source, extra=None):
    """Build a standardized message payload."""
    msg = {
        "case_id": case_id,
        "event_stage": event_stage,
        "status": status,
        "source": source,
        "timestamp": time.time(),
        "id": str(uuid.uuid4()),
    }
    if extra:
        msg.update(extra)
    return msg

class SimpleRabbitMQ:
    """Minimal RabbitMQ client for basic operations."""
    
    def __init__(self):
        self.connection = None
        self.ch = None
        self._return_callbacks = []
        self.connect()
    
    def connect(self):
        """Connect to RabbitMQ."""
        try:
            self.connection = pika.BlockingConnection(pika.ConnectionParameters('localhost'))
            self.ch = self.connection.channel()
            self.ch.exchange_declare(exchange=EXCHANGE_NAME, exchange_type=EXCHANGE_TYPE, durable=True)
            # Setup return handler for unroutable messages
            self.ch.add_on_return_callback(self._on_basic_return)
        except Exception as e:
            print(f"Failed to connect to RabbitMQ: {e}")
            raise
    
    def _on_basic_return(self, channel, method, properties, body):
        """Handle returned (unroutable) messages."""
        try:
            payload = json.loads(body.decode("utf-8"))
            info = {
                "routing_key": method.routing_key,
                "exchange": method.exchange,
                "reply_code": method.reply_code,
                "reply_text": method.reply_text,
                "payload": payload,
            }
            # Call registered callbacks
            for cb in self._return_callbacks:
                try:
                    cb(info)
                except Exception:
                    pass
        except Exception:
            pass
    
    def publish(self, routing_key: str, payload: dict, on_unroutable=None):
        """Publish a message with optional unroutable callback."""
        body = json.dumps(payload).encode("utf-8")
        if on_unroutable:
            self._return_callbacks.append(on_unroutable)
        try:
            self.ch.basic_publish(
                exchange=EXCHANGE_NAME,
                routing_key=routing_key,
                body=body,
                mandatory=True,  # Enable returns for unroutable messages
                properties=pika.BasicProperties(
                    content_type="application/json",
                    delivery_mode=2  # Make message persistent
                )
            )
            # For BlockingConnection, returned messages are delivered via process_data_events
            if on_unroutable:
                try:
                    # Give the broker time to deliver basic.return
                    self.connection.process_data_events(time_limit=1.0)
                except Exception:
                    pass
        except Exception as e:
            print(f"Failed to publish message: {e}")
            raise
        finally:
            if on_unroutable and on_unroutable in self._return_callbacks:
                try:
                    self._return_callbacks.remove(on_unroutable)
                except ValueError:
                    pass
    
    def declare_queue(self, queue_name, routing_key):
        """Declare and bind a queue."""
        self.ch.queue_declare(queue=queue_name, durable=True, auto_delete=False)
        self.ch.queue_bind(queue=queue_name, exchange=EXCHANGE_NAME, routing_key=routing_key)
    
    def consume_forever(self, queue_name, handler):
        """Consume messages from a queue forever."""
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
            print(f"[consume] stopping {queue_name}")
            self.ch.stop_consuming()
    
    def close(self):
        """Close the connection."""
        try:
            if self.connection and not self.connection.is_closed:
                self.connection.close()
        except Exception:
            pass

# Alias for backward compatibility
RabbitEventBus = SimpleRabbitMQ

def publish_one(quest_type: str) -> dict:
    """Publish a single quest."""
    import random
    points_by_type = {"gather": 5, "slay": 10, "escort": 15}
    difficulty_choices = [("easy", 1.0), ("medium", 2.0), ("hard", 3.5)]
    difficulty, work_sec = random.choice(difficulty_choices)
    points = points_by_type.get(quest_type, 5)
    quest_id = f"q-{int(time.time())}-{random.randint(100,999)}"
    payload = build_message(
        case_id=quest_id,
        event_stage="QUEST_ISSUED",
        status="NEW",
        source="game-master",
        extra={
            "quest_type": quest_type,
            "difficulty": difficulty,
            "work_sec": work_sec,
            "points": points,
            "weight": 1 if difficulty=="easy" else (2 if difficulty=="medium" else 4),
        },
    )
    bus = SimpleRabbitMQ()
    bus.publish(f"game.quest.{quest_type}", payload)
    bus.close()
    return payload

def publish_wave(count: int, delay: float, fixed_type: str = None) -> list:
    """Publish a wave of quests."""
    import random
    out = []
    for _ in range(count):
        quest_type = fixed_type or random.choice(["gather", "slay", "escort"])
        payload = publish_one(quest_type)
        out.append(payload)
        time.sleep(delay)
    return out