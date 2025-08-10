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
        
        # EDUCATIONAL: Broadcast raw message for RabbitMQ introspection
        try:
            # Import here to avoid circular imports
            from app.web_server import broadcast_raw_message
            broadcast_raw_message(routing_key, payload, source="publish")
        except Exception:
            pass  # Don't fail publish if broadcast fails
        
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
    
    def declare_queue(self, queue_name, routing_key, dlx_config=None):
        """Declare and bind a queue with optional DLQ configuration."""
        arguments = {}
        
        # Add dead-letter exchange configuration if provided
        if dlx_config:
            if dlx_config.get("dead_letter_exchange"):
                arguments["x-dead-letter-exchange"] = dlx_config["dead_letter_exchange"]
            if dlx_config.get("dead_letter_routing_key"):
                arguments["x-dead-letter-routing-key"] = dlx_config["dead_letter_routing_key"]
            if dlx_config.get("message_ttl"):
                arguments["x-message-ttl"] = dlx_config["message_ttl"]
                
        self.ch.queue_declare(queue=queue_name, durable=True, auto_delete=False, arguments=arguments)
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
    
    def setup_dlq_topology(self):
        """Set up comprehensive native RabbitMQ DLQ topology with specific queues for different failure types."""
        
        # 1. Main dead-letter exchange for all failure types
        self.ch.exchange_declare(exchange="game.dlx", exchange_type="topic", durable=True)
        
        # 2. Alternate exchange for unroutable messages  
        self.ch.exchange_declare(exchange="game.unroutable", exchange_type="fanout", durable=True)
        
        # 3. Specific DLQs for different failure reasons
        failure_queues = [
            # Retry stages with exponential backoff
            {"name": "game.quest.retry.1", "key": "retry.1", "ttl": 5000, "dlx": EXCHANGE_NAME, "dlx_key": "quest.retry"},
            {"name": "game.quest.retry.2", "key": "retry.2", "ttl": 30000, "dlx": EXCHANGE_NAME, "dlx_key": "quest.retry"},  
            {"name": "game.quest.retry.3", "key": "retry.3", "ttl": 300000, "dlx": EXCHANGE_NAME, "dlx_key": "quest.retry"},
            
            # Final DLQs by failure reason
            {"name": "game.quest.dlq.rejected", "key": "dlq.rejected", "description": "Messages rejected by consumers"},
            {"name": "game.quest.dlq.expired", "key": "dlq.expired", "description": "Messages that expired due to TTL"},
            {"name": "game.quest.dlq.maxlen", "key": "dlq.maxlen", "description": "Messages dropped due to queue length limits"},
            {"name": "game.quest.dlq.delivery_limit", "key": "dlq.delivery_limit", "description": "Messages exceeding delivery attempts"},
            {"name": "game.quest.dlq.poison", "key": "dlq.poison", "description": "Malformed or poison messages"},
            {"name": "game.quest.dlq.unroutable", "key": "dlq.unroutable", "description": "Messages that couldn't be routed"},
            
            # Quarantine for manual inspection
            {"name": "game.quest.quarantine", "key": "quarantine", "description": "Messages requiring manual intervention"},
        ]
        
        for queue_config in failure_queues:
            arguments = {}
            
            # Add TTL and dead-letter config for retry queues
            if "ttl" in queue_config:
                arguments["x-message-ttl"] = queue_config["ttl"]
                if "dlx" in queue_config:
                    arguments["x-dead-letter-exchange"] = queue_config["dlx"]
                    arguments["x-dead-letter-routing-key"] = queue_config["dlx_key"]
            
            self.ch.queue_declare(queue=queue_config["name"], durable=True, auto_delete=False, arguments=arguments)
            self.ch.queue_bind(queue=queue_config["name"], exchange="game.dlx", routing_key=queue_config["key"])
        
        # 4. Unroutable messages queue (bound to alternate exchange)
        self.ch.queue_declare(queue="game.quest.dlq.unroutable", durable=True, auto_delete=False)
        self.ch.queue_bind(queue="game.quest.dlq.unroutable", exchange="game.unroutable", routing_key="")
        
        # 5. Configure main exchange to use alternate exchange for unroutable messages
        try:
            self.ch.exchange_declare(
                exchange=EXCHANGE_NAME, 
                exchange_type=EXCHANGE_TYPE, 
                durable=True,
                arguments={"alternate-exchange": "game.unroutable"}
            )
        except Exception:
            pass  # Exchange might already exist
    
    def configure_main_queue_with_dlq(self, queue_name, routing_key, message_ttl=None):
        """Configure a main queue with dead-letter exchange for failures."""
        dlx_config = {
            "dead_letter_exchange": "game.dlx",
            "dead_letter_routing_key": "retry.1"  # First retry stage
        }
        
        # Optional: set per-queue TTL for automatic expiration
        if message_ttl:
            dlx_config["message_ttl"] = message_ttl
            
        self.declare_queue(queue_name, routing_key, dlx_config)
    
    def nack_to_dlq(self, delivery_tag, routing_key, requeue=False):
        """NACK a message to send it to DLQ (if configured with dead-letter exchange)."""
        self.ch.basic_nack(delivery_tag=delivery_tag, requeue=requeue)
    
    def inspect_dlq_message(self, body, properties):
        """Inspect x-death header to determine retry count and reason."""
        death_info = {"count": 0, "reason": "unknown", "original_routing_key": None}
        
        if properties and hasattr(properties, "headers") and properties.headers:
            x_death = properties.headers.get("x-death")
            if x_death:
                # x-death is a list of death records
                latest_death = x_death[0] if isinstance(x_death, list) else x_death
                if isinstance(latest_death, dict):
                    death_info["count"] = latest_death.get("count", 0)
                    death_info["reason"] = latest_death.get("reason", "unknown")
                    death_info["original_routing_key"] = latest_death.get("routing-keys", [None])[0]
        
        return death_info
    
    def republish_with_retry(self, payload, original_routing_key, retry_count=0, failure_reason="rejected"):
        """Republish message to appropriate retry queue or specific DLQ based on retry count and failure reason."""
        max_retries = 3
        
        if retry_count >= max_retries:
            # Route to specific DLQ based on failure reason
            dlq_routing_key = f"dlq.{failure_reason}"
            self.ch.basic_publish(
                exchange="game.dlx",
                routing_key=dlq_routing_key,
                body=json.dumps(payload),
                properties=pika.BasicProperties(
                    delivery_mode=2,  # Persistent
                    headers={
                        "retry_count": retry_count, 
                        "original_routing_key": original_routing_key,
                        "failure_reason": failure_reason,
                        "x-death-summary": f"Exhausted retries ({retry_count}) - reason: {failure_reason}"
                    }
                )
            )
            return f"dlq.{failure_reason}"
        else:
            # Send to appropriate retry queue
            retry_stage = retry_count + 1
            self.ch.basic_publish(
                exchange="game.dlx", 
                routing_key=f"retry.{retry_stage}",
                body=json.dumps(payload),
                properties=pika.BasicProperties(
                    delivery_mode=2,  # Persistent
                    headers={
                        "retry_count": retry_count, 
                        "original_routing_key": original_routing_key,
                        "failure_reason": failure_reason
                    }
                )
            )
            return f"retry.{retry_stage}"
    
    def send_to_dlq(self, payload, routing_key, reason="rejected", additional_headers=None):
        """Send message directly to specific DLQ based on failure reason."""
        headers = {
            "failure_reason": reason,
            "original_routing_key": routing_key,
            "dlq_timestamp": time.time()
        }
        if additional_headers:
            headers.update(additional_headers)
            
        # Map reasons to routing keys
        reason_mapping = {
            "rejected": "dlq.rejected",
            "expired": "dlq.expired", 
            "maxlen": "dlq.maxlen",
            "delivery_limit": "dlq.delivery_limit",
            "poison": "dlq.poison",
            "unroutable": "dlq.unroutable",
            "quarantine": "quarantine"
        }
        
        dlq_routing_key = reason_mapping.get(reason, "dlq.rejected")
        
        self.ch.basic_publish(
            exchange="game.dlx",
            routing_key=dlq_routing_key,
            body=json.dumps(payload),
            properties=pika.BasicProperties(
                delivery_mode=2,  # Persistent
                headers=headers
            )
        )
        return dlq_routing_key
    
    def publish_with_mandatory(self, routing_key, payload, on_unroutable=None):
        """Publish with mandatory=True to catch unroutable messages."""
        def handle_return(channel, method, properties, body):
            # Message was unroutable - send to unroutable DLQ
            unroutable_payload = json.loads(body.decode("utf-8"))
            unroutable_payload["return_info"] = {
                "reply_code": method.reply_code,
                "reply_text": method.reply_text,
                "exchange": method.exchange,
                "routing_key": method.routing_key
            }
            self.send_to_dlq(unroutable_payload, routing_key, "unroutable")
            
            if on_unroutable:
                try:
                    on_unroutable({
                        "routing_key": method.routing_key,
                        "reply_code": method.reply_code,
                        "reply_text": method.reply_text,
                        "payload": unroutable_payload
                    })
                except Exception:
                    pass
        
        # Add temporary return handler
        if handle_return not in [cb for cb in self._return_callbacks]:
            self.ch.add_on_return_callback(handle_return)
        
        try:
            self.ch.basic_publish(
                exchange=EXCHANGE_NAME,
                routing_key=routing_key,
                body=json.dumps(payload),
                properties=pika.BasicProperties(delivery_mode=2),
                mandatory=True  # Catch unroutable messages
            )
        except Exception as e:
            # Fallback: send to unroutable DLQ
            self.send_to_dlq(payload, routing_key, "unroutable", {"publish_error": str(e)})
    
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