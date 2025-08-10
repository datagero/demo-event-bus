package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"demo-event-bus-workers/chaos"
	"demo-event-bus-workers/consumer"
)

// Server manages Go workers and chaos actions
type Server struct {
	workers      map[string]*consumer.Worker
	chaosManager *chaos.Manager
	mu           sync.RWMutex
	rabbitURL    string
	webhookURL   string
}

// NewServer creates a new worker server
func NewServer(rabbitURL, webhookURL string) *Server {
	return &Server{
		workers:      make(map[string]*consumer.Worker),
		chaosManager: chaos.NewManager(),
		rabbitURL:    rabbitURL,
		webhookURL:   webhookURL,
	}
}

// StartWorker starts a new worker
func (s *Server) StartWorker(config consumer.WorkerConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set server URLs in config
	config.RabbitURL = s.rabbitURL
	config.WebhookURL = s.webhookURL

	// Stop existing worker if it exists
	if existingWorker, exists := s.workers[config.PlayerName]; exists {
		log.Printf("🔄 [Server] Stopping existing worker: %s", config.PlayerName)
		existingWorker.Stop()
		s.chaosManager.UnregisterWorker(config.PlayerName)
	}

	// Create and start new worker
	worker := consumer.NewWorker(config)
	err := worker.Start()
	if err != nil {
		return err
	}

	// Register worker
	s.workers[config.PlayerName] = worker
	s.chaosManager.RegisterWorker(config.PlayerName, worker)

	log.Printf("✅ [Server] Started worker: %s", config.PlayerName)
	return nil
}

// StopWorker stops a worker
func (s *Server) StopWorker(playerName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	worker, exists := s.workers[playerName]
	if !exists {
		return nil // Already stopped
	}

	err := worker.Stop()
	delete(s.workers, playerName)
	s.chaosManager.UnregisterWorker(playerName)

	log.Printf("🛑 [Server] Stopped worker: %s", playerName)
	return err
}

// PauseWorker pauses a worker
func (s *Server) PauseWorker(playerName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	worker, exists := s.workers[playerName]
	if !exists {
		return nil
	}

	worker.Pause()
	return nil
}

// ResumeWorker resumes a worker
func (s *Server) ResumeWorker(playerName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	worker, exists := s.workers[playerName]
	if !exists {
		return nil
	}

	worker.Resume()
	return nil
}

// GetStatus returns server status
func (s *Server) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workerNames := make([]string, 0, len(s.workers))
	for name := range s.workers {
		workerNames = append(workerNames, name)
	}

	return map[string]interface{}{
		"worker_count": len(s.workers),
		"workers":      workerNames,
		"chaos":        s.chaosManager.GetStatus(),
	}
}

// HTTP handlers

func (s *Server) handleStartWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config consumer.WorkerConfig
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err = s.StartWorker(config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Worker started",
		"player":  config.PlayerName,
	})
}

func (s *Server) handleStopWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Player string `json:"player"`
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err = s.StopWorker(req.Player)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Worker stopped",
		"player":  req.Player,
	})
}

func (s *Server) handlePauseWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Player string `json:"player"`
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err = s.PauseWorker(req.Player)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Worker paused",
		"player":  req.Player,
	})
}

func (s *Server) handleResumeWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Player string `json:"player"`
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err = s.ResumeWorker(req.Player)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Worker resumed",
		"player":  req.Player,
	})
}

func (s *Server) handleChaos(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Get chaos status
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.chaosManager.GetStatus())
		return
	}

	if r.Method == http.MethodPost {
		// Trigger chaos action
		var req struct {
			Action       chaos.Action `json:"action"`
			TargetPlayer string       `json:"target_player"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		err = s.chaosManager.TriggerAction(req.Action, req.TargetPlayer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"message": "Chaos action triggered",
			"action":  req.Action,
			"target":  req.TargetPlayer,
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.GetStatus())
}

func main() {
	// Command line flags
	var (
		port       = flag.String("port", "8001", "HTTP server port")
		rabbitURL  = flag.String("rabbit", "amqp://guest:guest@localhost:5672/", "RabbitMQ URL")
		webhookURL = flag.String("webhook", "http://localhost:8000/api/go-workers/webhook/events", "Python webhook URL")
	)
	flag.Parse()

	// Create server
	server := NewServer(*rabbitURL, *webhookURL)

	// Set up HTTP routes
	http.HandleFunc("/start", server.handleStartWorker)
	http.HandleFunc("/stop", server.handleStopWorker)
	http.HandleFunc("/pause", server.handlePauseWorker)
	http.HandleFunc("/resume", server.handleResumeWorker)
	http.HandleFunc("/chaos", server.handleChaos)
	http.HandleFunc("/status", server.handleStatus)

	// Health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Start auto-chaos in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.chaosManager.AutoTrigger(ctx)

	// Start HTTP server
	log.Printf("🚀 [Go Server] Starting on port %s", *port)
	log.Printf("🐰 [Go Server] RabbitMQ: %s", *rabbitURL)
	log.Printf("🔗 [Go Server] Webhook: %s", *webhookURL)

	// Graceful shutdown
	httpServer := &http.Server{Addr: ":" + *port}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("🛑 [Go Server] Shutting down...")

	// Stop all workers
	server.mu.Lock()
	for name, worker := range server.workers {
		log.Printf("🛑 [Go Server] Stopping worker: %s", name)
		worker.Stop()
	}
	server.mu.Unlock()

	// Shutdown HTTP server
	httpServer.Shutdown(context.Background())
	log.Println("✅ [Go Server] Shutdown complete")
}
