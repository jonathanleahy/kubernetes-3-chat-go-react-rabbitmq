package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Message represents a chat message
type Message struct {
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Sender    string    `json:"sender,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	PodName   string    `json:"podName"`
}

// Client represents a connected WebSocket client
type Client struct {
	ID   string
	Conn *websocket.Conn
}

// ServerStatus represents the server's current status
type ServerStatus struct {
	Status        string         `json:"status"`
	Version       string         `json:"version"`
	Endpoints     []string       `json:"endpoints"`
	ActiveClients int32          `json:"activeClients"`
	RabbitMQ      RabbitMQStatus `json:"rabbitmq"`
	ServerTime    time.Time      `json:"serverTime"`
	PodName       string         `json:"podName"`
}

// RabbitMQStatus represents RabbitMQ connection status
type RabbitMQStatus struct {
	Connected bool   `json:"connected"`
	URL       string `json:"url"`
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 10 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	clients           = make(map[*Client]bool)
	clientsMutex      sync.RWMutex
	globalClientCount int32
	startTime         = time.Now()
)

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func main() {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		var err error
		podName, err = os.Hostname()
		if err != nil {
			podName = "unknown-pod"
		}
	}
	log.Printf("Starting chat backend server on pod: %s", podName)

	// Enhanced root endpoint
	http.HandleFunc("/", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		rabbitmqURL := os.Getenv("RABBITMQ_URL")
		if rabbitmqURL == "" {
			rabbitmqURL = "amqp://guest:guest@rabbitmq-service:5672/"
		}

		clientCount := atomic.LoadInt32(&globalClientCount)
		log.Printf("Current global client count: %d", clientCount)

		status := ServerStatus{
			Status:        "running",
			Version:       "1.0.0",
			Endpoints:     []string{"/", "/health", "/ws", "/status", "/debug"},
			ActiveClients: clientCount,
			RabbitMQ: RabbitMQStatus{
				Connected: true,
				URL:       strings.Replace(rabbitmqURL, "guest:guest@", "***:***@", 1),
			},
			ServerTime: time.Now(),
			PodName:    podName,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}))

	// Status endpoint
	http.HandleFunc("/status", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		clientCount := atomic.LoadInt32(&globalClientCount)
		log.Printf("Current global client count (status): %d", clientCount)

		status := struct {
			Status      string    `json:"status"`
			PodName     string    `json:"podName"`
			Uptime      string    `json:"uptime"`
			ClientCount int32     `json:"clientCount"`
			ServerTime  time.Time `json:"serverTime"`
		}{
			Status:      "operational",
			PodName:     podName,
			Uptime:      time.Since(startTime).String(),
			ClientCount: clientCount,
			ServerTime:  time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}))

	// Debug endpoint
	http.HandleFunc("/debug", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		debug := struct {
			PodName       string           `json:"podName"`
			GlobalClients int32            `json:"globalClients"`
			LocalClients  int              `json:"localClients"`
			Memory        runtime.MemStats `json:"memory"`
		}{
			PodName:       podName,
			GlobalClients: atomic.LoadInt32(&globalClientCount),
			LocalClients:  len(clients),
			Memory:        memStats,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(debug)
	}))

	// Health check endpoint
	http.HandleFunc("/health", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		clientCount := atomic.LoadInt32(&globalClientCount)
		health := struct {
			Status    string `json:"status"`
			PodName   string `json:"podName"`
			Clients   int32  `json:"connectedClients"`
			Timestamp string `json:"timestamp"`
		}{
			Status:    "healthy",
			PodName:   podName,
			Clients:   clientCount,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health)
	}))

	// RabbitMQ setup
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@rabbitmq-service:5672/"
	}
	log.Printf("Using RabbitMQ URL: %s", rabbitmqURL)

	var conn *amqp.Connection
	var err error
	for i := 0; i < 30; i++ {
		log.Printf("Attempting to connect to RabbitMQ (attempt %d)...", i+1)
		conn, err = amqp.Dial(rabbitmqURL)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to RabbitMQ: %v", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ after retries: %v", err)
	}
	defer conn.Close()
	log.Printf("Successfully connected to RabbitMQ")

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	defer ch.Close()

	// Message exchange
	exchangeName := "chat_messages"
	err = ch.ExchangeDeclare(
		exchangeName, // name
		"fanout",     // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// Queue setup
	q, err := ch.QueueDeclare(
		"",    // name (empty for auto-generated)
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Bind queue to exchange
	err = ch.QueueBind(
		q.Name,       // queue name
		"",           // routing key
		exchangeName, // exchange
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	// Message consumer
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	// Handle messages
	go func() {
		for msg := range msgs {
			log.Printf("Received message from RabbitMQ: %s", string(msg.Body))

			var chatMsg Message
			if err := json.Unmarshal(msg.Body, &chatMsg); err != nil {
				log.Printf("Error parsing message: %v", err)
				continue
			}

			// Broadcast all messages received from RabbitMQ
			broadcastMessage(msg.Body)
		}
	}()

	// WebSocket endpoint
	http.HandleFunc("/ws", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("New WebSocket connection request from %s on pod %s", r.RemoteAddr, podName)
		handleWebSocket(w, r, ch, exchangeName, podName)
	}))

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	serverAddr := fmt.Sprintf(":%s", port)
	log.Printf("Server starting on %s (Pod: %s)", serverAddr, podName)
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request, ch *amqp.Channel, exchangeName, podName string) {
	log.Printf("New WebSocket connection from %s", r.RemoteAddr)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	client := &Client{
		ID:   fmt.Sprintf("%s-%d", podName, time.Now().UnixNano()),
		Conn: conn,
	}

	clientsMutex.Lock()
	clients[client] = true
	clientCount := len(clients)
	clientsMutex.Unlock()

	newCount := atomic.AddInt32(&globalClientCount, 1)
	log.Printf("Client connected. Total clients: %d, Local clients: %d", newCount, clientCount)

	// Send welcome message
	welcomeMsg := Message{
		Type:      "system",
		Content:   fmt.Sprintf("Connected to chat server (Pod: %s)", podName),
		Timestamp: time.Now(),
		PodName:   podName,
	}
	welcomeBytes, _ := json.Marshal(welcomeMsg)
	err = conn.WriteMessage(websocket.TextMessage, welcomeBytes)
	if err != nil {
		log.Printf("Error sending welcome message: %v", err)
	}

	defer func() {
		clientsMutex.Lock()
		delete(clients, client)
		remainingClients := len(clients)
		clientsMutex.Unlock()
		remainingCount := atomic.AddInt32(&globalClientCount, -1)
		log.Printf("Client disconnected. Total clients: %d, Local clients: %d", remainingCount, remainingClients)
		conn.Close()
	}()

	for {
		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}

		log.Printf("Received message from %s: %s", client.ID, string(rawMsg))

		var msg Message
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}

		msg.Sender = client.ID
		msg.Timestamp = time.Now()
		msg.Type = "message"
		msg.PodName = podName

		// Only publish to RabbitMQ, don't broadcast directly
		msgBytes, _ := json.Marshal(msg)
		log.Printf("Publishing to RabbitMQ: %s", string(msgBytes))

		err = ch.Publish(
			exchangeName,
			"",
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        msgBytes,
			},
		)
		if err != nil {
			log.Printf("Failed to publish to RabbitMQ: %v", err)
			continue
		}
		log.Printf("Successfully published to RabbitMQ")
	}
}

func broadcastMessage(message []byte) {
	var msg Message
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("Error parsing broadcast message: %v", err)
		return
	}

	clientsMutex.RLock()
	defer clientsMutex.RUnlock()

	log.Printf("Broadcasting message to %d clients", len(clients))

	for client := range clients {
		err := client.Conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf("Error sending to client %s: %v", client.ID, err)
		} else {
			log.Printf("Successfully sent message to client %s", client.ID)
		}
	}
}
