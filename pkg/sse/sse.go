// Package sse provides a Server-Sent Events (SSE) broker for managing
// client connections, broadcasting events, and automatic heartbeats.
package sse

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Event represents a Server-Sent Event.
type Event struct {
	// ID is the optional event ID for reconnection tracking.
	ID string
	// Type is the event type (maps to SSE "event:" field).
	Type string
	// Data is the event payload.
	Data []byte
	// Retry is the optional reconnection time in milliseconds.
	Retry int
}

// Format serializes the Event into SSE wire format.
func (e *Event) Format() []byte {
	var buf []byte
	if e.ID != "" {
		buf = append(buf, []byte("id: "+e.ID+"\n")...)
	}
	if e.Type != "" {
		buf = append(buf, []byte("event: "+e.Type+"\n")...)
	}
	if e.Retry > 0 {
		buf = append(buf, []byte("retry: "+strconv.Itoa(e.Retry)+"\n")...)
	}
	buf = append(buf, []byte("data: ")...)
	buf = append(buf, e.Data...)
	buf = append(buf, []byte("\n\n")...)
	return buf
}

// Client represents a connected SSE client.
type Client struct {
	// ID is the unique identifier for this client.
	ID string
	// Channel is used to send events to this client.
	Channel chan []byte
	// LastEventID is the last event ID received, for reconnection.
	LastEventID string
	// CreatedAt is when the client connected.
	CreatedAt time.Time
}

// Config holds the configuration for the SSE Broker.
type Config struct {
	// BufferSize is the channel buffer size per client.
	BufferSize int
	// HeartbeatInterval is the interval for sending heartbeat events.
	HeartbeatInterval time.Duration
	// MaxClients is the maximum number of concurrent clients.
	MaxClients int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		BufferSize:        100,
		HeartbeatInterval: 30 * time.Second,
		MaxClients:        1000,
	}
}

// Broker manages SSE client connections and broadcasts events.
type Broker struct {
	config *Config

	clients   map[string]*Client
	clientsMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	clientCount atomic.Int64
}

// NewBroker creates a new SSE Broker with the given configuration.
// If config is nil, DefaultConfig is used.
func NewBroker(config *Config) *Broker {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	b := &Broker{
		config:  config,
		clients: make(map[string]*Client),
		ctx:     ctx,
		cancel:  cancel,
	}

	b.wg.Add(1)
	go b.heartbeatLoop()

	return b
}

// AddClient upgrades an HTTP connection to SSE and registers a new client.
// It blocks until the client disconnects or the broker is closed.
func (b *Broker) AddClient(w http.ResponseWriter, r *http.Request) (*Client, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported by response writer")
	}

	if b.config.MaxClients > 0 &&
		int(b.clientCount.Load()) >= b.config.MaxClients {
		return nil, fmt.Errorf("maximum client limit reached: %d", b.config.MaxClients)
	}

	clientID := r.Header.Get("X-Client-ID")
	if clientID == "" {
		clientID = fmt.Sprintf("client-%d", time.Now().UnixNano())
	}

	lastEventID := r.Header.Get("Last-Event-ID")

	client := &Client{
		ID:          clientID,
		Channel:     make(chan []byte, b.config.BufferSize),
		LastEventID: lastEventID,
		CreatedAt:   time.Now(),
	}

	b.clientsMu.Lock()
	b.clients[clientID] = client
	b.clientsMu.Unlock()
	b.clientCount.Add(1)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// Stream events until disconnect
	go func() {
		<-r.Context().Done()
		b.RemoveClient(clientID)
	}()

	go func() {
		for {
			select {
			case data, ok := <-client.Channel:
				if !ok {
					return
				}
				_, _ = w.Write(data)
				flusher.Flush()
			case <-b.ctx.Done():
				return
			case <-r.Context().Done():
				return
			}
		}
	}()

	return client, nil
}

// RemoveClient disconnects and removes a client by ID.
func (b *Broker) RemoveClient(id string) {
	b.clientsMu.Lock()
	client, exists := b.clients[id]
	if exists {
		close(client.Channel)
		delete(b.clients, id)
		b.clientCount.Add(-1)
	}
	b.clientsMu.Unlock()
}

// Broadcast sends an event to all connected clients.
func (b *Broker) Broadcast(event *Event) {
	data := event.Format()

	b.clientsMu.RLock()
	defer b.clientsMu.RUnlock()

	for _, client := range b.clients {
		select {
		case client.Channel <- data:
		default:
			// Client buffer full, skip
		}
	}
}

// SendTo sends an event to a specific client by ID.
func (b *Broker) SendTo(clientID string, event *Event) error {
	data := event.Format()

	b.clientsMu.RLock()
	client, exists := b.clients[clientID]
	b.clientsMu.RUnlock()

	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}

	select {
	case client.Channel <- data:
		return nil
	default:
		return fmt.Errorf("client channel full: %s", clientID)
	}
}

// ClientCount returns the number of connected clients.
func (b *Broker) ClientCount() int {
	return int(b.clientCount.Load())
}

// Close stops the broker and disconnects all clients.
func (b *Broker) Close() {
	b.cancel()
	b.wg.Wait()

	b.clientsMu.Lock()
	for id, client := range b.clients {
		close(client.Channel)
		delete(b.clients, id)
	}
	b.clientsMu.Unlock()
	b.clientCount.Store(0)
}

// heartbeatLoop sends periodic heartbeat events to all clients.
func (b *Broker) heartbeatLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.Broadcast(&Event{
				Type: "heartbeat",
				Data: []byte(`{"type":"heartbeat"}`),
			})
		}
	}
}

// ServeHTTP implements http.Handler for the SSE broker endpoint.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	client, err := b.AddClient(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Block until the client disconnects
	<-r.Context().Done()
	_ = client
}
