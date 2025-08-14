package clients

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/streadway/amqp"
)

// RabbitMQClient handles direct RabbitMQ operations for message publishing
type RabbitMQClient struct {
	url           string
	managementURL string
	username      string
	password      string
	connection    *amqp.Connection
	channel       *amqp.Channel
	httpClient    *http.Client
}

// NewRabbitMQClient creates a new RabbitMQ client
func NewRabbitMQClient(url string) *RabbitMQClient {
	client := &RabbitMQClient{
		url:           url,
		managementURL: "http://localhost:15672/api",
		username:      "guest",
		password:      "guest",
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}
	if err := client.connect(); err != nil {
		log.Printf("❌ [RabbitMQ] Failed to connect: %v", err)
	}
	return client
}

// connect establishes connection to RabbitMQ
func (c *RabbitMQClient) connect() error {
	var err error
	c.connection, err = amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	c.channel, err = c.connection.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare the main exchange
	err = c.channel.ExchangeDeclare(
		"game.skill",
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	log.Printf("🐰 [RabbitMQ] Connected and exchange declared")
	return nil
}

// ensureConnection checks if connection and channel are healthy, reconnects if needed
func (c *RabbitMQClient) ensureConnection() error {
	// Check if we need to establish initial connection
	if c.connection == nil || c.channel == nil {
		return c.connect()
	}

	// Check if connection is still alive
	if c.connection.IsClosed() {
		log.Printf("🔄 [RabbitMQ] Connection is closed, reconnecting...")
		return c.reconnect()
	}

	// Check if channel is still usable by trying a simple operation
	if err := c.channel.ExchangeDeclarePassive(
		"game.skill",
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,   // arguments
	); err != nil {
		log.Printf("🔄 [RabbitMQ] Channel appears unhealthy, reconnecting: %v", err)
		return c.reconnect()
	}

	return nil
}

// reconnect closes existing connections and establishes new ones
func (c *RabbitMQClient) reconnect() error {
	// Close existing connections gracefully
	if c.channel != nil {
		c.channel.Close()
		c.channel = nil
	}
	if c.connection != nil {
		c.connection.Close()
		c.connection = nil
	}

	// Establish new connection
	return c.connect()
}

// PublishMessage publishes a single message
func (c *RabbitMQClient) PublishMessage(routingKey string, payload map[string]interface{}) error {
	// Ensure we have a healthy connection and channel
	if err := c.ensureConnection(); err != nil {
		return fmt.Errorf("failed to ensure connection: %w", err)
	}

	// Build message with timestamp and ID
	message := map[string]interface{}{
		"id":        fmt.Sprintf("q-%d-%d", time.Now().Unix(), time.Now().Nanosecond()),
		"type":      routingKey,
		"payload":   payload,
		"timestamp": time.Now().Unix(),
		"points":    c.getPointsForType(routingKey),
	}

	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = c.channel.Publish(
		"game.skill", // exchange (game-specific)
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // make message persistent
		},
	)

	if err != nil {
		// If publish fails, try to reconnect once and retry
		log.Printf("⚠️ [RabbitMQ] Publish failed, attempting reconnection: %v", err)
		if reconnectErr := c.reconnect(); reconnectErr != nil {
			return fmt.Errorf("failed to publish message (reconnect failed): %w", err)
		}

		// Retry publish after reconnection
		err = c.channel.Publish(
			"game.skill", // exchange (game-specific)
			routingKey,   // routing key
			false,        // mandatory
			false,        // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         body,
				DeliveryMode: amqp.Persistent, // make message persistent
			},
		)

		if err != nil {
			return fmt.Errorf("failed to publish message after reconnect: %w", err)
		}
	}

	log.Printf("📨 [RabbitMQ] Published message to %s: %s", routingKey, message["id"])
	return nil
}

// PublishWave publishes multiple messages
func (c *RabbitMQClient) PublishWave(routingKey string, count int, delay time.Duration) error {
	for i := 0; i < count; i++ {
		payload := map[string]interface{}{
			"wave_index": i + 1,
			"wave_total": count,
		}

		if err := c.PublishMessage(routingKey, payload); err != nil {
			return fmt.Errorf("failed to publish wave message %d: %w", i+1, err)
		}

		if delay > 0 && i < count-1 {
			time.Sleep(delay)
		}
	}

	log.Printf("🌊 [RabbitMQ] Published wave of %d messages to %s", count, routingKey)
	return nil
}

// GetQueueInfo retrieves information about a queue
func (c *RabbitMQClient) GetQueueInfo(queueName string) (map[string]interface{}, error) {
	if err := c.ensureConnection(); err != nil {
		return nil, fmt.Errorf("failed to ensure connection: %w", err)
	}

	queue, err := c.channel.QueueInspect(queueName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect queue %s: %w", queueName, err)
	}

	return map[string]interface{}{
		"name":      queue.Name,
		"messages":  queue.Messages,
		"consumers": queue.Consumers,
	}, nil
}

// ListQueues lists all queues (basic implementation)
func (c *RabbitMQClient) ListQueues() ([]map[string]interface{}, error) {
	// For a more complete implementation, we'd use RabbitMQ Management API
	// For now, return the known skill queues
	skillQueues := []string{
		"game.skill.gather.q",
		"game.skill.escort.q",
		"game.skill.slay.q",
		"game.skill.heal.q",
	}

	var queues []map[string]interface{}
	for _, queueName := range skillQueues {
		info, err := c.GetQueueInfo(queueName)
		if err != nil {
			// Queue might not exist, skip it
			continue
		}
		queues = append(queues, info)
	}

	return queues, nil
}

// getPointsForType returns points value for different message types
func (c *RabbitMQClient) getPointsForType(messageType string) int {
	pointsMap := map[string]int{
		"gather": 5,
		"escort": 10,
		"slay":   15,
		"heal":   8,
	}

	if points, exists := pointsMap[messageType]; exists {
		return points
	}
	return 5 // default points
}

// PurgeQueue removes all messages from a queue
func (c *RabbitMQClient) PurgeQueue(queueName string) error {
	if err := c.ensureConnection(); err != nil {
		return fmt.Errorf("failed to ensure connection: %w", err)
	}

	messageCount, err := c.channel.QueuePurge(queueName, false)
	if err != nil {
		return fmt.Errorf("failed to purge queue %s: %w", queueName, err)
	}

	log.Printf("🧹 [RabbitMQ] Purged %d messages from queue %s", messageCount, queueName)
	return nil
}

// DeleteQueue completely deletes a queue from RabbitMQ
func (c *RabbitMQClient) DeleteQueue(queueName string) error {
	// URL-encode the queue name for the Management API
	encodedQueueName := url.QueryEscape(queueName)
	endpoint := fmt.Sprintf("/queues/%%2F/%s", encodedQueueName)
	apiURL := fmt.Sprintf("%s%s", c.managementURL, endpoint)

	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	// Add basic auth
	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Add("Authorization", "Basic "+auth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Queue doesn't exist, which is fine for our purposes
		log.Printf("🗑️ [RabbitMQ] Queue %s already doesn't exist", queueName)
		return nil
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete queue %s: status %d, body: %s", queueName, resp.StatusCode, string(body))
	}

	log.Printf("🗑️ [RabbitMQ] Successfully deleted queue %s", queueName)
	return nil
}

// CreateExchange creates an exchange via RabbitMQ Management API
func (c *RabbitMQClient) CreateExchange(exchangeName string, exchangeType string, durable bool, autoDelete bool, internal bool, arguments map[string]interface{}) error {
	encodedExchangeName := url.QueryEscape(exchangeName)
	endpoint := fmt.Sprintf("/exchanges/%%2F/%s", encodedExchangeName)
	apiURL := fmt.Sprintf("%s%s", c.managementURL, endpoint)

	exchangeData := map[string]interface{}{
		"type":        exchangeType,
		"durable":     durable,
		"auto_delete": autoDelete,
		"internal":    internal,
		"arguments":   arguments,
	}

	jsonData, err := json.Marshal(exchangeData)
	if err != nil {
		return fmt.Errorf("failed to marshal exchange data: %w", err)
	}

	req, err := http.NewRequest("PUT", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Add("Authorization", "Basic "+auth)
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create exchange: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create exchange %s: status %d, body: %s", exchangeName, resp.StatusCode, body)
	}

	log.Printf("🔄 [RabbitMQ] Successfully created exchange %s (%s)", exchangeName, exchangeType)
	return nil
}

// CreateQueue creates a queue via RabbitMQ Management API
func (c *RabbitMQClient) CreateQueue(queueName string, durable bool, autoDelete bool, arguments map[string]interface{}) error {
	encodedQueueName := url.QueryEscape(queueName)
	endpoint := fmt.Sprintf("/queues/%%2F/%s", encodedQueueName)
	apiURL := fmt.Sprintf("%s%s", c.managementURL, endpoint)

	queueData := map[string]interface{}{
		"durable":     durable,
		"auto_delete": autoDelete,
		"arguments":   arguments,
	}

	jsonData, err := json.Marshal(queueData)
	if err != nil {
		return fmt.Errorf("failed to marshal queue data: %w", err)
	}

	req, err := http.NewRequest("PUT", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Add("Authorization", "Basic "+auth)
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create queue %s: status %d, body: %s", queueName, resp.StatusCode, body)
	}

	log.Printf("📦 [RabbitMQ] Successfully created queue %s", queueName)
	return nil
}

// BindQueue creates a binding between a queue and exchange via RabbitMQ Management API
func (c *RabbitMQClient) BindQueue(queueName string, exchangeName string, routingKey string, arguments map[string]interface{}) error {
	encodedQueueName := url.QueryEscape(queueName)
	encodedExchangeName := url.QueryEscape(exchangeName)
	endpoint := fmt.Sprintf("/bindings/%%2F/e/%s/q/%s", encodedExchangeName, encodedQueueName)
	apiURL := fmt.Sprintf("%s%s", c.managementURL, endpoint)

	bindingData := map[string]interface{}{
		"routing_key": routingKey,
		"arguments":   arguments,
	}

	jsonData, err := json.Marshal(bindingData)
	if err != nil {
		return fmt.Errorf("failed to marshal binding data: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Add("Authorization", "Basic "+auth)
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to bind queue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to bind queue %s to exchange %s: status %d, body: %s", queueName, exchangeName, resp.StatusCode, body)
	}

	log.Printf("🔗 [RabbitMQ] Successfully bound queue %s to exchange %s with routing key %s", queueName, exchangeName, routingKey)
	return nil
}

// Close closes the RabbitMQ connection
func (c *RabbitMQClient) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.connection != nil {
		return c.connection.Close()
	}
	return nil
}

// RabbitMQ Management API integration for educational RabbitMQ-direct queries

// managementAPIRequest makes a request to the RabbitMQ Management API
func (c *RabbitMQClient) managementAPIRequest(endpoint string) ([]byte, error) {
	apiURL := fmt.Sprintf("%s%s", c.managementURL, endpoint)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add basic auth
	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Add("Authorization", "Basic "+auth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// GetQueuesFromAPI retrieves queue information directly from RabbitMQ Management API
func (c *RabbitMQClient) GetQueuesFromAPI() ([]map[string]interface{}, error) {
	body, err := c.managementAPIRequest("/queues")
	if err != nil {
		return nil, err
	}

	var queues []map[string]interface{}
	if err := json.Unmarshal(body, &queues); err != nil {
		return nil, fmt.Errorf("failed to unmarshal queues response: %w", err)
	}

	return queues, nil
}

// GetConsumersFromAPI retrieves consumer information directly from RabbitMQ Management API
func (c *RabbitMQClient) GetConsumersFromAPI() ([]map[string]interface{}, error) {
	body, err := c.managementAPIRequest("/consumers")
	if err != nil {
		return nil, err
	}

	var consumers []map[string]interface{}
	if err := json.Unmarshal(body, &consumers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal consumers response: %w", err)
	}

	return consumers, nil
}

// GetExchangesFromAPI retrieves exchange information directly from RabbitMQ Management API
func (c *RabbitMQClient) GetExchangesFromAPI() ([]map[string]interface{}, error) {
	body, err := c.managementAPIRequest("/exchanges")
	if err != nil {
		return nil, err
	}

	var exchanges []map[string]interface{}
	if err := json.Unmarshal(body, &exchanges); err != nil {
		return nil, fmt.Errorf("failed to unmarshal exchanges response: %w", err)
	}

	return exchanges, nil
}

// GetBindingsFromAPI retrieves binding information directly from RabbitMQ Management API
func (c *RabbitMQClient) GetBindingsFromAPI() ([]map[string]interface{}, error) {
	body, err := c.managementAPIRequest("/bindings")
	if err != nil {
		return nil, err
	}

	var bindings []map[string]interface{}
	if err := json.Unmarshal(body, &bindings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bindings response: %w", err)
	}

	return bindings, nil
}

// PeekQueueMessages retrieves messages from a queue using RabbitMQ Management API
func (c *RabbitMQClient) PeekQueueMessages(queueName string, count int) ([]map[string]interface{}, error) {
	// URL-encode the queue name
	encodedQueueName := url.QueryEscape(queueName)
	endpoint := fmt.Sprintf("/queues/%%2F/%s/get", encodedQueueName)

	requestBody := map[string]interface{}{
		"count":    count,
		"ackmode":  "ack_requeue_true",
		"encoding": "auto",
		"truncate": 50000,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	apiURL := fmt.Sprintf("%s%s", c.managementURL, endpoint)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Add("Authorization", "Basic "+auth)
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []map[string]interface{}{}, nil // Queue doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var messages []map[string]interface{}
	if err := json.Unmarshal(body, &messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal messages response: %w", err)
	}

	return messages, nil
}

// DeriveMetricsFromRabbitMQ calculates all UI metrics directly from RabbitMQ state
func (c *RabbitMQClient) DeriveMetricsFromRabbitMQ() (map[string]interface{}, error) {
	// Get queues and consumers from RabbitMQ
	queues, err := c.GetQueuesFromAPI()
	if err != nil {
		return nil, fmt.Errorf("failed to get queues: %w", err)
	}

	consumers, err := c.GetConsumersFromAPI()
	if err != nil {
		return nil, fmt.Errorf("failed to get consumers: %w", err)
	}

	// Calculate metrics directly from RabbitMQ state
	metrics := map[string]interface{}{
		"source":          "direct_rabbitmq_go_client",
		"timestamp":       time.Now().Unix(),
		"queue_stats":     make(map[string]interface{}),
		"consumer_stats":  make(map[string]interface{}),
		"total_pending":   0,
		"total_unacked":   0,
		"total_consumers": len(consumers),
		"per_type":        make(map[string]interface{}),
		"worker_roster":   make(map[string]interface{}),
	}

	// Process queue statistics
	totalPending := 0
	totalUnacked := 0
	perType := make(map[string]map[string]int)

	for _, queue := range queues {
		queueName, ok := queue["name"].(string)
		if !ok {
			continue
		}

		// Filter game-related queues only
		if !c.isGameQueue(queueName) {
			continue
		}

		ready := c.getIntField(queue, "messages_ready")
		unacked := c.getIntField(queue, "messages_unacknowledged")

		totalPending += ready
		totalUnacked += unacked

		// Extract quest type from queue name
		questType := c.extractQuestTypeFromQueue(queueName)
		if questType != "" {
			if perType[questType] == nil {
				perType[questType] = make(map[string]int)
			}
			perType[questType]["pending"] += ready
			perType[questType]["accepted"] += unacked
		}

		metrics["queue_stats"].(map[string]interface{})[queueName] = map[string]interface{}{
			"ready":   ready,
			"unacked": unacked,
			"total":   ready + unacked,
		}
	}

	metrics["total_pending"] = totalPending
	metrics["total_unacked"] = totalUnacked
	metrics["per_type"] = perType

	log.Printf("📊 [RabbitMQ-Go] Derived metrics: %d queues, %d total pending, %d consumers",
		len(metrics["queue_stats"].(map[string]interface{})), totalPending, len(consumers))

	return metrics, nil
}

// Helper functions
func (c *RabbitMQClient) isGameQueue(queueName string) bool {
	gameQueuePrefixes := []string{"game.", "web."}
	for _, prefix := range gameQueuePrefixes {
		if len(queueName) >= len(prefix) && queueName[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func (c *RabbitMQClient) extractQuestTypeFromQueue(queueName string) string {
	// Extract quest type from queue names like "game.skill.slay.q" -> "slay"
	if len(queueName) > 11 && queueName[:11] == "game.skill." {
		parts := queueName[11:] // Remove "game.skill."
		if dotIndex := len(parts) - 2; dotIndex > 0 && parts[dotIndex:] == ".q" {
			return parts[:dotIndex] // Return everything before ".q"
		}
	}
	return ""
}

func (c *RabbitMQClient) getIntField(data map[string]interface{}, field string) int {
	if val, ok := data[field]; ok {
		if intVal, ok := val.(float64); ok {
			return int(intVal)
		}
	}
	return 0
}

// DeleteExchange deletes an exchange using the RabbitMQ Management API
func (c *RabbitMQClient) DeleteExchange(exchangeName string) error {
	// URL-encode the exchange name for the Management API
	encodedExchangeName := url.QueryEscape(exchangeName)
	endpoint := fmt.Sprintf("/exchanges/%%2F/%s", encodedExchangeName)
	apiURL := fmt.Sprintf("%s%s", c.managementURL, endpoint)

	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete exchange request: %w", err)
	}

	// Add basic auth (consistent with other methods)
	auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
	req.Header.Add("Authorization", "Basic "+auth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute delete exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Exchange doesn't exist, which is fine for our purposes
		log.Printf("🗑️ [RabbitMQ] Exchange %s already doesn't exist", exchangeName)
		return nil
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("🗑️ [RabbitMQ] Successfully deleted exchange %s", exchangeName)
	return nil
}

// GetConnectionsFromAPI gets all RabbitMQ connections
func (c *RabbitMQClient) GetConnectionsFromAPI() ([]map[string]interface{}, error) {
	apiURL := fmt.Sprintf("%s/connections", c.managementURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get connections request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute get connections request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get connections failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read connections response body: %w", err)
	}

	var connections []map[string]interface{}
	if err := json.Unmarshal(body, &connections); err != nil {
		return nil, fmt.Errorf("failed to unmarshal connections response: %w", err)
	}

	return connections, nil
}

// CloseConnection closes a specific RabbitMQ connection
func (c *RabbitMQClient) CloseConnection(connectionName string) error {
	// URL encode the connection name
	encodedName := url.QueryEscape(connectionName)
	apiURL := fmt.Sprintf("%s/connections/%s", c.managementURL, encodedName)

	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create close connection request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute close connection request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Connection doesn't exist, which is fine for our purposes
		log.Printf("🔌 [RabbitMQ] Connection %s already doesn't exist", connectionName)
		return nil
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("close connection failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("🔌 [RabbitMQ] Successfully closed connection %s", connectionName)
	return nil
}
