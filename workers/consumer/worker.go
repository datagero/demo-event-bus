package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"demo-event-bus-workers/broker"

	amqp "github.com/rabbitmq/amqp091-go"
)

// WorkerConfig holds configuration for a worker
type WorkerConfig struct {
	PlayerName      string   `json:"player"`
	Skills          []string `json:"skills"`
	FailPct         float64  `json:"fail_pct"`
	SpeedMultiplier float64  `json:"speed_multiplier"`
	WorkerCount     int      `json:"workers"`
	RoutingMode     string   `json:"routing_mode"` // "player" or "skill"
	RabbitURL       string   `json:"rabbit_url"`
	WebhookURL      string   `json:"webhook_url"` // Python server callback
}

// Worker represents a Go-based message worker
type Worker struct {
	Config  WorkerConfig // Made public for status reporting
	client  *broker.Client
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	paused  bool
	pauseMu sync.RWMutex
}

// NewWorker creates a new worker instance
func NewWorker(config WorkerConfig) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	return &Worker{
		Config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the worker goroutines
func (w *Worker) Start() error {
	var err error
	w.client, err = broker.NewClient(w.Config.RabbitURL)
	if err != nil {
		return fmt.Errorf("failed to create broker client: %w", err)
	}

	// Set prefetch based on worker count
	err = w.client.SetQoS(w.Config.WorkerCount)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// Declare queues based on routing mode
	if w.Config.RoutingMode == "player" {
		// Per-player queue
		queueName := fmt.Sprintf("game.player.%s.q", w.Config.PlayerName)
		err = w.client.DeclareQueue(queueName, "game.quest.*")
		if err != nil {
			return err
		}

		// Start workers for this queue
		w.startWorkers(queueName)
	} else {
		// Skill-based queues (shared)
		for _, skill := range w.Config.Skills {
			queueName := fmt.Sprintf("game.skill.%s.q", skill)
			routingKey := fmt.Sprintf("game.quest.%s", skill)

			err = w.client.DeclareQueue(queueName, routingKey)
			if err != nil {
				return err
			}

			// Start workers for this skill queue
			w.startSkillWorkers(queueName, skill)
		}
	}

	// Notify Python server that worker is online
	w.notifyStatus("online")

	log.Printf("🚀 [Go Worker] %s started with %d workers, skills: %v",
		w.Config.PlayerName, w.Config.WorkerCount, w.Config.Skills)

	return nil
}

// Stop gracefully stops the worker
func (w *Worker) Stop() error {
	log.Printf("🛑 [Go Worker] Stopping %s...", w.Config.PlayerName)

	w.cancel()  // Cancel context to stop all goroutines
	w.wg.Wait() // Wait for all goroutines to finish

	if w.client != nil {
		w.client.Close()
	}

	// Notify Python server that worker is offline
	w.notifyStatus("offline")

	log.Printf("✅ [Go Worker] %s stopped", w.Config.PlayerName)
	return nil
}

// Pause pauses the worker
func (w *Worker) Pause() {
	w.pauseMu.Lock()
	defer w.pauseMu.Unlock()
	w.paused = true
	w.notifyStatus("paused")
	log.Printf("⏸️ [Go Worker] %s paused", w.Config.PlayerName)
}

// Resume resumes the worker
func (w *Worker) Resume() {
	w.pauseMu.Lock()
	defer w.pauseMu.Unlock()
	w.paused = false
	w.notifyStatus("online")
	log.Printf("▶️ [Go Worker] %s resumed", w.Config.PlayerName)
}

// isPaused returns whether the worker is paused
func (w *Worker) isPaused() bool {
	w.pauseMu.RLock()
	defer w.pauseMu.RUnlock()
	return w.paused
}

// startWorkers starts worker goroutines for a specific queue
func (w *Worker) startWorkers(queueName string) {
	for i := 0; i < w.Config.WorkerCount; i++ {
		w.wg.Add(1)
		go w.workerLoop(queueName, i, "")
	}
}

// startSkillWorkers starts worker goroutines for a specific skill queue
func (w *Worker) startSkillWorkers(queueName string, skill string) {
	for i := 0; i < w.Config.WorkerCount; i++ {
		w.wg.Add(1)
		go w.workerLoop(queueName, i, skill)
	}
}

// workerLoop is the main message processing loop for a worker goroutine
func (w *Worker) workerLoop(queueName string, workerID int, skill string) {
	defer w.wg.Done()

	log.Printf("🏃 [Go Worker] %s-worker-%d started on queue %s",
		w.Config.PlayerName, workerID, queueName)

	// Message handler with enhanced DLQ support
	handler := func(delivery amqp.Delivery) bool {
		// Check if we should stop
		select {
		case <-w.ctx.Done():
			log.Printf("🛑 [Go Worker] %s stopping, requeuing message", w.Config.PlayerName)
			delivery.Nack(false, true) // NACK with requeue for infrastructure issues
			return true                // We handled the NACK manually
		default:
		}

		// Check if paused
		if w.isPaused() {
			log.Printf("⏸️ [Go Worker] %s paused, requeuing message", w.Config.PlayerName)
			delivery.Nack(false, true) // NACK with requeue for infrastructure issues
			return true                // We handled the NACK manually
		}

		// Process the message and handle ACK/NACK based on the type of failure
		success := w.processMessage(delivery)

		if success {
			delivery.Ack(false) // ACK successful processing
		} else {
			// Message processing failed - send to DLQ (no requeue)
			log.Printf("💀 [Go Worker] %s: Message processing failed, sending to DLQ", w.Config.PlayerName)
			delivery.Nack(false, false) // NACK without requeue -> DLQ
		}

		return true // We handled ACK/NACK manually
	}

	// Create a unique consumer tag that includes skill to avoid conflicts
	var consumerTag string
	if skill != "" {
		consumerTag = fmt.Sprintf("%s-%s-worker-%d", w.Config.PlayerName, skill, workerID)
	} else {
		consumerTag = fmt.Sprintf("%s-worker-%d", w.Config.PlayerName, workerID)
	}

	// Start consuming (this blocks until context is cancelled)
	err := w.client.ConsumeWithTagAndContext(w.ctx, queueName, consumerTag, handler)
	if err != nil && err != context.Canceled {
		log.Printf("❌ [Go Worker] Consumer error for %s: %v", w.Config.PlayerName, err)
	}
}

// processMessage processes a single message
func (w *Worker) processMessage(delivery amqp.Delivery) bool {
	// --- Enhanced Debug Logging ---
	log.Printf("📬 [Go Worker Debug] %s received raw message: %s", w.Config.PlayerName, string(delivery.Body))

	msg, err := broker.ParseMessage(delivery)
	if err != nil {
		log.Printf("❌ [Go Worker] Failed to parse message: %v. Raw body: %s", err, string(delivery.Body))
		return true // Ack anyway to avoid poison message loop
	}

	// --- Enhanced Debug Logging ---
	log.Printf("✅ [Go Worker Debug] %s parsed message: %+v", w.Config.PlayerName, msg)
	log.Printf("ℹ️ [Go Worker Debug] %s checking for skill '%s' in %v", w.Config.PlayerName, msg.QuestType, w.Config.Skills)

	// IGNORE completion messages - these should not be processed as new quests
	if msg.EventStage == "QUEST_COMPLETED" || msg.EventStage == "QUEST_FAILED" {
		log.Printf("📄 [Go Worker] %s ignoring completion message %s (EventStage: %s)", w.Config.PlayerName, msg.CaseID, msg.EventStage)
		return true // ACK the completion message but don't process it
	}

	// Check if this worker should handle this quest type
	if w.Config.RoutingMode == "skill" && !w.hasSkill(msg.QuestType) {
		log.Printf("⚠️ [Go Worker] %s skipping %s (no skill)", w.Config.PlayerName, msg.QuestType)
		return false // Nack and requeue for correct worker
	}

	// Reduced logging: only log for debugging
	// log.Printf("📨 [Go Worker] %s accepted %s (%s)",
	//	w.Config.PlayerName, msg.CaseID, msg.QuestType)

	// Notify Python server about message acceptance
	w.notifyMessageEvent("accept", msg)

	// Simulate work (scaled by speed multiplier)
	workDuration := time.Duration(msg.WorkSec*w.Config.SpeedMultiplier*1000) * time.Millisecond

	select {
	case <-time.After(workDuration):
		// Work completed
	case <-w.ctx.Done():
		// Worker stopped during work
		log.Printf("🛑 [Go Worker] %s stopped during work on %s", w.Config.PlayerName, msg.CaseID)
		return false // Nack and requeue
	}

	// Check if paused after work (simulate pause during processing)
	if w.isPaused() {
		log.Printf("⏸️ [Go Worker] %s paused after work, requeuing %s", w.Config.PlayerName, msg.CaseID)
		return false // Nack and requeue
	}

	// Determine outcome based on fail percentage
	success := rand.Float64() >= w.Config.FailPct

	// For quest failures, we want to send them to DLQ instead of requeuing
	if !success {
		log.Printf("💀 [Go Worker] %s quest %s failed - sending to DLQ", w.Config.PlayerName, msg.CaseID)

		// Create failed result message
		failedMsg := broker.Message{
			CaseID:     msg.CaseID,
			EventStage: "QUEST_FAILED",
			Status:     "FAILED",
			Source:     fmt.Sprintf("go-worker:%s", w.Config.PlayerName),
			QuestType:  msg.QuestType,
			Difficulty: msg.Difficulty,
			Points:     0,
			Player:     w.Config.PlayerName,
		}

		// Publish failed result
		failedRoutingKey := fmt.Sprintf("game.quest.%s.fail", msg.QuestType)
		err = w.client.Publish(w.ctx, failedRoutingKey, failedMsg)
		if err != nil {
			log.Printf("❌ [Go Worker] Failed to publish failed result: %v", err)
			return false // Nack to retry
		}

		// Notify Python server about failure
		w.notifyMessageEvent("failed", failedMsg)

		// Return a special value to indicate DLQ (we'll modify the handler)
		return false // This will be handled as DLQ in our modified handler
	}

	// Create success result message
	resultMsg := broker.Message{
		CaseID:     msg.CaseID,
		EventStage: "QUEST_COMPLETED",
		Status:     "SUCCESS",
		Source:     fmt.Sprintf("go-worker:%s", w.Config.PlayerName),
		QuestType:  msg.QuestType,
		Difficulty: msg.Difficulty,
		Points:     msg.Points,
		Player:     w.Config.PlayerName,
	}

	// Publish result
	routingKey := fmt.Sprintf("game.quest.%s.done", msg.QuestType)

	err = w.client.Publish(w.ctx, routingKey, resultMsg)
	if err != nil {
		log.Printf("❌ [Go Worker] Failed to publish result: %v", err)
		return false // Nack to retry
	}

	// Reduced logging: only log for debugging
	// log.Printf("✅ [Go Worker] %s completed %s (+%d pts)",
	//	w.Config.PlayerName, msg.CaseID, resultMsg.Points)

	// Notify Python server about completion
	w.notifyMessageEvent("completed", resultMsg)

	return true // Ack the message
}

// hasSkill checks if the worker has a specific skill
func (w *Worker) hasSkill(questType string) bool {
	for _, skill := range w.Config.Skills {
		if skill == questType {
			return true
		}
	}
	return false
}

// notifyStatus sends status updates to the Python server
func (w *Worker) notifyStatus(status string) {
	if w.Config.WebhookURL == "" {
		return
	}

	payload := map[string]interface{}{
		"type":   "worker_status",
		"player": w.Config.PlayerName,
		"status": status,
		"source": "go-worker",
	}

	w.sendWebhook(payload)
}

// notifyMessageEvent sends message events to the Python server
func (w *Worker) notifyMessageEvent(eventType string, msg broker.Message) {
	if w.Config.WebhookURL == "" {
		return
	}

	payload := map[string]interface{}{
		"type":    "message_event",
		"event":   eventType,
		"player":  w.Config.PlayerName,
		"message": msg,
		"source":  "go-worker",
	}

	w.sendWebhook(payload)
}

// sendWebhook sends a webhook to the Python server
func (w *Worker) sendWebhook(payload map[string]interface{}) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("❌ [Go Worker] Failed to marshal webhook: %v", err)
		return
	}

	// Send async to avoid blocking
	go func() {
		resp, err := http.Post(w.Config.WebhookURL, "application/json",
			strings.NewReader(string(jsonData)))
		if err != nil {
			log.Printf("❌ [Go Worker] Webhook failed: %v", err)
			return
		}
		defer resp.Body.Close()
	}()
}
