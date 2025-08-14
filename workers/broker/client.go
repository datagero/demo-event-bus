package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName = "game.skill"
	ExchangeType = "topic"
)

// Client wraps RabbitMQ connection and channel
type Client struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

// Message represents a quest message
type Message struct {
	CaseID     string                 `json:"case_id"`
	EventStage string                 `json:"event_stage"`
	Status     string                 `json:"status"`
	Source     string                 `json:"source"`
	QuestType  string                 `json:"quest_type"`
	Difficulty float64                `json:"difficulty"`
	WorkSec    float64                `json:"work_sec"`
	Points     int                    `json:"points"`
	Weight     int                    `json:"weight"`
	Player     string                 `json:"player,omitempty"`
	Payload    map[string]interface{} `json:"payload"`
}

// NewClient creates a new RabbitMQ client
func NewClient(rabbitURL string) (*Client, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare the exchange
	err = ch.ExchangeDeclare(
		ExchangeName, // name
		ExchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	return &Client{
		conn:    conn,
		channel: ch,
		closed:  false,
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true

	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// DeclareQueue creates and binds a queue with dead letter exchange configuration
func (c *Client) DeclareQueue(queueName, routingKey string) error {
	// Configure dead letter exchange for failed messages
	args := amqp.Table{
		"x-dead-letter-exchange":    "game.dlx",   // Dead letter exchange
		"x-dead-letter-routing-key": "dlq.failed", // Route failed messages to "dlq.failed"
	}

	_, err := c.channel.QueueDeclare(
		queueName, // name
		true,      // durable (survives broker restart)
		false,     // auto-delete
		false,     // exclusive
		false,     // no-wait
		args,      // arguments (with DLX configuration)
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", queueName, err)
	}

	err = c.channel.QueueBind(
		queueName,    // queue name
		routingKey,   // routing key
		ExchangeName, // exchange
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue %s: %w", queueName, err)
	}

	return nil
}

// SetQoS sets the prefetch count for the channel
func (c *Client) SetQoS(prefetchCount int) error {
	return c.channel.Qos(prefetchCount, 0, false)
}

// Publish publishes a message to the exchange
func (c *Client) Publish(ctx context.Context, routingKey string, msg Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return c.channel.PublishWithContext(
		ctx,
		ExchangeName, // exchange
		routingKey,   // routing key
		true,         // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // make message persistent
			Body:         body,
			Timestamp:    time.Now(),
		},
	)
}

// Consume starts consuming messages from a queue
func (c *Client) Consume(queueName string, handler func(amqp.Delivery) bool) error {
	return c.ConsumeWithTag(queueName, "", handler)
}

// ConsumeWithTag starts consuming messages from a queue with a specific consumer tag
func (c *Client) ConsumeWithTag(queueName string, consumerTag string, handler func(amqp.Delivery) bool) error {
	return c.ConsumeWithTagAndContext(context.Background(), queueName, consumerTag, handler)
}

// ConsumeWithTagAndContext starts consuming messages from a queue with a specific consumer tag and respects context cancellation
func (c *Client) ConsumeWithTagAndContext(ctx context.Context, queueName string, consumerTag string, handler func(amqp.Delivery) bool) error {
	deliveries, err := c.channel.Consume(
		queueName,   // queue
		consumerTag, // consumer tag
		false,       // auto-ack (we'll ack manually)
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Printf("🔄 [Go Consumer] Waiting for messages on queue: %s", queueName)

	// Process messages until context is cancelled
	for {
		select {
		case <-ctx.Done():
			log.Printf("🛑 [Go Consumer] Context cancelled, stopping consumer for queue: %s", queueName)
			// Cancel the consumer
			c.channel.Cancel(consumerTag, false)
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				log.Printf("🔚 [Go Consumer] Delivery channel closed for queue: %s", queueName)
				return nil
			}
			// Call handler - it now handles ACK/NACK manually
			// We only fall back to auto-handling if handler returns false
			if !handler(delivery) {
				// Handler wants to nack/requeue (fallback for legacy behavior)
				delivery.Nack(false, true)
			}
			// If handler returns true, it has handled ACK/NACK manually
		}
	}
}

// ParseMessage parses a delivery into a Message struct.
// It is resilient to two formats:
// 1. New format (flat): { "case_id": "...", "quest_type": "..." }
// 2. Legacy Python format (nested): { "payload": { "case_id": "...", "quest_type": "..." } }
func ParseMessage(delivery amqp.Delivery) (Message, error) {
	var msg Message

	// Try to unmarshal into the top-level struct first (new format)
	err := json.Unmarshal(delivery.Body, &msg)
	if err == nil && msg.QuestType != "" {
		return msg, nil // Successfully parsed new format
	}

	// If that fails or results in an empty QuestType, try the legacy format
	var legacyMsg struct {
		Payload Message `json:"payload"`
	}
	err = json.Unmarshal(delivery.Body, &legacyMsg)
	if err != nil {
		// If both fail, return the original error
		return msg, fmt.Errorf("failed to unmarshal message in any known format: %w", err)
	}

	// Return the nested message from the legacy payload
	return legacyMsg.Payload, nil
}
