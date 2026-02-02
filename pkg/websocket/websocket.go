// Package websocket provides a WebSocket hub with support for rooms,
// read/write pumps, ping/pong, and message broadcasting.
package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"
)

// Message represents a WebSocket message.
type Message struct {
	// Type is the message type identifier.
	Type string `json:"type"`
	// Room is the optional room target.
	Room string `json:"room,omitempty"`
	// Data is the message payload.
	Data json.RawMessage `json:"data,omitempty"`
	// Sender is the client ID that sent this message.
	Sender string `json:"sender,omitempty"`
}

// Config holds configuration for the WebSocket Hub.
type Config struct {
	ReadBufferSize  int
	WriteBufferSize int
	PingInterval    time.Duration
	PongWait        time.Duration
	WriteWait       time.Duration
	MaxMessageSize  int64
	AllowedOrigins  []string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    54 * time.Second,
		PongWait:        60 * time.Second,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024, // 512KB
		AllowedOrigins:  []string{"*"},
	}
}

// Client represents a WebSocket client connection.
type Client struct {
	id     string
	conn   *ws.Conn
	send   chan []byte
	hub    *Hub
	rooms  map[string]bool
	mu     sync.Mutex
	closed bool
}

// ID returns the client identifier.
func (c *Client) ID() string {
	return c.id
}

// Send queues data to be sent to the client.
func (c *Client) Send(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("client is closed")
	}

	select {
	case c.send <- data:
		return nil
	default:
		return fmt.Errorf("client send buffer full")
	}
}

// Close closes the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.send)
	return c.conn.Close()
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) writePump(config *Config, ctx context.Context) {
	ticker := time.NewTicker(config.PingInterval)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(config.WriteWait))
			if !ok {
				_ = c.conn.WriteMessage(ws.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(ws.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(config.WriteWait))
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
func (c *Client) readPump(config *Config, ctx context.Context) {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(config.MaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(config.PongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(config.PongWait))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, msgBytes, err := c.conn.ReadMessage()
			if err != nil {
				if ws.IsUnexpectedCloseError(
					err,
					ws.CloseGoingAway,
					ws.CloseAbnormalClosure,
				) {
					// unexpected close
				}
				return
			}

			var msg Message
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				continue
			}
			msg.Sender = c.id

			if c.hub.onMessage != nil {
				c.hub.onMessage(c, &msg)
			}
		}
	}
}

// Room represents a group of clients.
type Room struct {
	name    string
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewRoom creates a new room.
func NewRoom(name string) *Room {
	return &Room{
		name:    name,
		clients: make(map[string]*Client),
	}
}

// Hub manages WebSocket connections, rooms, and broadcasting.
type Hub struct {
	config   *Config
	upgrader ws.Upgrader

	clients   map[string]*Client
	clientsMu sync.RWMutex

	rooms   map[string]*Room
	roomsMu sync.RWMutex

	register   chan *Client
	unregister chan *Client

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	onMessage func(*Client, *Message)

	clientIDCounter int64
	counterMu       sync.Mutex
}

// NewHub creates a new WebSocket Hub. If config is nil, DefaultConfig is used.
func NewHub(config *Config) *Hub {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	h := &Hub{
		config: config,
		upgrader: ws.Upgrader{
			ReadBufferSize:  config.ReadBufferSize,
			WriteBufferSize: config.WriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				if len(config.AllowedOrigins) == 0 {
					return true
				}
				origin := r.Header.Get("Origin")
				for _, allowed := range config.AllowedOrigins {
					if allowed == "*" || allowed == origin {
						return true
					}
				}
				return false
			},
		},
		clients:    make(map[string]*Client),
		rooms:      make(map[string]*Room),
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
		ctx:        ctx,
		cancel:     cancel,
	}

	h.wg.Add(1)
	go h.run()

	return h
}

// OnMessage sets the handler for incoming messages.
func (h *Hub) OnMessage(handler func(*Client, *Message)) {
	h.onMessage = handler
}

// run processes register/unregister events.
func (h *Hub) run() {
	defer h.wg.Done()

	for {
		select {
		case client := <-h.register:
			h.clientsMu.Lock()
			h.clients[client.id] = client
			h.clientsMu.Unlock()

		case client := <-h.unregister:
			h.clientsMu.Lock()
			if _, ok := h.clients[client.id]; ok {
				delete(h.clients, client.id)
				// Remove from all rooms
				h.roomsMu.Lock()
				for _, room := range h.rooms {
					room.mu.Lock()
					delete(room.clients, client.id)
					room.mu.Unlock()
				}
				h.roomsMu.Unlock()
			}
			h.clientsMu.Unlock()

		case <-h.ctx.Done():
			return
		}
	}
}

// ServeWS handles WebSocket connection upgrade requests.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) (*Client, error) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket upgrade failed: %w", err)
	}

	h.counterMu.Lock()
	h.clientIDCounter++
	clientID := fmt.Sprintf("ws-%d", h.clientIDCounter)
	h.counterMu.Unlock()

	headerID := r.Header.Get("X-Client-ID")
	if headerID != "" {
		clientID = headerID
	}

	client := &Client{
		id:    clientID,
		conn:  conn,
		send:  make(chan []byte, 256),
		hub:   h,
		rooms: make(map[string]bool),
	}

	h.register <- client

	go client.writePump(h.config, h.ctx)
	go client.readPump(h.config, h.ctx)

	return client, nil
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	for _, client := range h.clients {
		_ = client.Send(data)
	}
	return nil
}

// SendToRoom sends a message to all clients in a room.
func (h *Hub) SendToRoom(roomName string, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	h.roomsMu.RLock()
	room, exists := h.rooms[roomName]
	h.roomsMu.RUnlock()

	if !exists {
		return fmt.Errorf("room not found: %s", roomName)
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	for _, client := range room.clients {
		_ = client.Send(data)
	}
	return nil
}

// JoinRoom adds a client to a room.
func (h *Hub) JoinRoom(clientID, roomName string) error {
	h.clientsMu.RLock()
	client, exists := h.clients[clientID]
	h.clientsMu.RUnlock()

	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}

	h.roomsMu.Lock()
	room, roomExists := h.rooms[roomName]
	if !roomExists {
		room = NewRoom(roomName)
		h.rooms[roomName] = room
	}
	h.roomsMu.Unlock()

	room.mu.Lock()
	room.clients[clientID] = client
	room.mu.Unlock()

	client.mu.Lock()
	client.rooms[roomName] = true
	client.mu.Unlock()

	return nil
}

// LeaveRoom removes a client from a room.
func (h *Hub) LeaveRoom(clientID, roomName string) error {
	h.roomsMu.RLock()
	room, exists := h.rooms[roomName]
	h.roomsMu.RUnlock()

	if !exists {
		return fmt.Errorf("room not found: %s", roomName)
	}

	room.mu.Lock()
	delete(room.clients, clientID)
	room.mu.Unlock()

	h.clientsMu.RLock()
	client, clientExists := h.clients[clientID]
	h.clientsMu.RUnlock()

	if clientExists {
		client.mu.Lock()
		delete(client.rooms, roomName)
		client.mu.Unlock()
	}

	return nil
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return len(h.clients)
}

// RoomCount returns the number of rooms.
func (h *Hub) RoomCount() int {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()
	return len(h.rooms)
}

// RoomClientCount returns the number of clients in a room.
func (h *Hub) RoomClientCount(roomName string) int {
	h.roomsMu.RLock()
	room, exists := h.rooms[roomName]
	h.roomsMu.RUnlock()

	if !exists {
		return 0
	}

	room.mu.RLock()
	defer room.mu.RUnlock()
	return len(room.clients)
}

// Close stops the hub and disconnects all clients.
func (h *Hub) Close() {
	h.cancel()
	h.wg.Wait()

	h.clientsMu.Lock()
	for _, client := range h.clients {
		_ = client.Close()
	}
	h.clients = make(map[string]*Client)
	h.clientsMu.Unlock()
}
