package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, 1024, config.ReadBufferSize)
	assert.Equal(t, 1024, config.WriteBufferSize)
	assert.Equal(t, 54*time.Second, config.PingInterval)
	assert.Equal(t, 60*time.Second, config.PongWait)
	assert.Equal(t, 10*time.Second, config.WriteWait)
	assert.Equal(t, int64(512*1024), config.MaxMessageSize)
	assert.Equal(t, []string{"*"}, config.AllowedOrigins)
}

func TestNewHub(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name: "custom config",
			config: &Config{
				ReadBufferSize:  2048,
				WriteBufferSize: 2048,
				PingInterval:    30 * time.Second,
				PongWait:        40 * time.Second,
				WriteWait:       5 * time.Second,
				MaxMessageSize:  1024 * 1024,
				AllowedOrigins:  []string{"http://localhost"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub(tt.config)
			require.NotNil(t, hub)
			assert.Equal(t, 0, hub.ClientCount())
			assert.Equal(t, 0, hub.RoomCount())
			hub.Close()
		})
	}
}

func TestHub_ServeWS(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := hub.ServeWS(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Give the hub time to register the client
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect two clients
	conn1, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn1.Close() }()

	conn2, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn2.Close() }()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, hub.ClientCount())

	// Broadcast a message
	msg := &Message{
		Type: "test",
		Data: json.RawMessage(`{"hello":"world"}`),
	}
	err = hub.Broadcast(msg)
	require.NoError(t, err)

	// Both clients should receive the message
	for _, conn := range []*ws.Conn{conn1, conn2} {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msgBytes, err := conn.ReadMessage()
		require.NoError(t, err)

		var received Message
		err = json.Unmarshal(msgBytes, &received)
		require.NoError(t, err)
		assert.Equal(t, "test", received.Type)
	}
}

func TestHub_Rooms(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn1, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn1.Close() }()

	conn2, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn2.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Get client IDs
	hub.clientsMu.RLock()
	var clientIDs []string
	for id := range hub.clients {
		clientIDs = append(clientIDs, id)
	}
	hub.clientsMu.RUnlock()
	require.Len(t, clientIDs, 2)

	// Join room
	err = hub.JoinRoom(clientIDs[0], "room1")
	require.NoError(t, err)
	assert.Equal(t, 1, hub.RoomCount())
	assert.Equal(t, 1, hub.RoomClientCount("room1"))

	// Join same room with another client
	err = hub.JoinRoom(clientIDs[1], "room1")
	require.NoError(t, err)
	assert.Equal(t, 2, hub.RoomClientCount("room1"))

	// Leave room
	err = hub.LeaveRoom(clientIDs[0], "room1")
	require.NoError(t, err)
	assert.Equal(t, 1, hub.RoomClientCount("room1"))

	// Non-existent room
	assert.Equal(t, 0, hub.RoomClientCount("nonexistent"))
}

func TestHub_SendToRoom(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn1, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn1.Close() }()

	conn2, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn2.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Get first client ID and join room
	hub.clientsMu.RLock()
	var firstID string
	for id := range hub.clients {
		firstID = id
		break
	}
	hub.clientsMu.RUnlock()

	err = hub.JoinRoom(firstID, "exclusive")
	require.NoError(t, err)

	// Send to room - should not error
	msg := &Message{Type: "room-msg", Data: json.RawMessage(`{"in":"room"}`)}
	err = hub.SendToRoom("exclusive", msg)
	require.NoError(t, err)

	// Send to non-existent room - should error
	err = hub.SendToRoom("ghost-room", msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "room not found")
}

func TestHub_JoinRoom_NonExistentClient(t *testing.T) {
	hub := NewHub(nil)
	defer hub.Close()

	err := hub.JoinRoom("nonexistent", "room1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client not found")
}

func TestHub_LeaveRoom_NonExistentRoom(t *testing.T) {
	hub := NewHub(nil)
	defer hub.Close()

	err := hub.LeaveRoom("client1", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "room not found")
}

func TestMessage_JSON(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "simple message",
			msg: Message{
				Type: "chat",
				Data: json.RawMessage(`{"text":"hello"}`),
			},
		},
		{
			name: "room message",
			msg: Message{
				Type:   "chat",
				Room:   "general",
				Data:   json.RawMessage(`{"text":"hi"}`),
				Sender: "user1",
			},
		},
		{
			name: "empty data",
			msg: Message{
				Type: "ping",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.msg)
			require.NoError(t, err)

			var decoded Message
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Type, decoded.Type)
			assert.Equal(t, tt.msg.Room, decoded.Room)
			assert.Equal(t, tt.msg.Sender, decoded.Sender)
		})
	}
}

func TestClient_SendClosed(t *testing.T) {
	client := &Client{
		id:     "test",
		send:   make(chan []byte, 10),
		closed: true,
		rooms:  make(map[string]bool),
	}

	err := client.Send([]byte("hello"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client is closed")
}

func TestNewRoom(t *testing.T) {
	room := NewRoom("test-room")
	assert.Equal(t, "test-room", room.name)
	assert.NotNil(t, room.clients)
	assert.Empty(t, room.clients)
}

func TestServeWS_UpgradeFailure(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectError    bool
		expectedStatus int
	}{
		{
			name:           "non-GET request fails upgrade",
			method:         http.MethodPost,
			expectError:    true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing WebSocket headers fails upgrade",
			method:         http.MethodGet,
			expectError:    true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub(nil)
			defer hub.Close()

			// Create a request without proper WebSocket upgrade headers
			req := httptest.NewRequest(tt.method, "/ws", nil)
			w := httptest.NewRecorder()

			client, err := hub.ServeWS(w, req)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
				assert.Contains(t, err.Error(), "websocket upgrade failed")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestServeWS_OriginCheckFailure(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		expectReject   bool
	}{
		{
			name:           "wildcard allows any origin",
			allowedOrigins: []string{"*"},
			requestOrigin:  "http://evil.com",
			expectReject:   false,
		},
		{
			name:           "specific origin matches",
			allowedOrigins: []string{"http://localhost:8080"},
			requestOrigin:  "http://localhost:8080",
			expectReject:   false,
		},
		{
			name:           "specific origin rejects mismatch",
			allowedOrigins: []string{"http://localhost:8080"},
			requestOrigin:  "http://evil.com",
			expectReject:   true,
		},
		{
			name:           "empty allowed origins allows all",
			allowedOrigins: []string{},
			requestOrigin:  "http://any.com",
			expectReject:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub(&Config{
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
				PingInterval:    1 * time.Hour,
				PongWait:        1 * time.Hour,
				WriteWait:       10 * time.Second,
				MaxMessageSize:  512 * 1024,
				AllowedOrigins:  tt.allowedOrigins,
			})
			defer hub.Close()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = hub.ServeWS(w, r)
			}))
			defer server.Close()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			dialer := ws.Dialer{}
			header := http.Header{}
			header.Set("Origin", tt.requestOrigin)

			conn, _, err := dialer.Dial(wsURL, header)
			if tt.expectReject {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if conn != nil {
					_ = conn.Close()
				}
			}
		})
	}
}

func TestServeWS_CustomClientID(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	header := http.Header{}
	header.Set("X-Client-ID", "custom-client-123")

	conn, _, err := ws.DefaultDialer.Dial(wsURL, header)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Verify client ID from hub's client map
	hub.clientsMu.RLock()
	var foundClientID string
	for id := range hub.clients {
		foundClientID = id
	}
	hub.clientsMu.RUnlock()

	assert.Equal(t, "custom-client-123", foundClientID)
}

func TestClient_SendBufferFull(t *testing.T) {
	// Create a client with a very small buffer
	client := &Client{
		id:     "test-buffer",
		send:   make(chan []byte, 1), // buffer of 1
		closed: false,
		rooms:  make(map[string]bool),
	}

	// Fill the buffer
	err := client.Send([]byte("first"))
	assert.NoError(t, err)

	// Next send should fail with buffer full
	err = client.Send([]byte("second"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "send buffer full")
}

func TestClient_CloseIdempotent(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Get the client
	hub.clientsMu.RLock()
	var client *Client
	for _, c := range hub.clients {
		client = c
		break
	}
	hub.clientsMu.RUnlock()

	require.NotNil(t, client)

	// First close should succeed
	err = client.Close()
	assert.NoError(t, err)

	// Second close should be idempotent (no error, no panic)
	err = client.Close()
	assert.NoError(t, err)

	_ = conn.Close()
}

func TestHub_OnMessage(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	receivedMsgs := make(chan *Message, 10)
	hub.OnMessage(func(c *Client, m *Message) {
		receivedMsgs <- m
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Send a message from client to server
	msg := Message{
		Type: "test-msg",
		Data: json.RawMessage(`{"key":"value"}`),
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)

	err = conn.WriteMessage(ws.TextMessage, msgBytes)
	require.NoError(t, err)

	// Wait for the message to be received
	select {
	case received := <-receivedMsgs:
		assert.Equal(t, "test-msg", received.Type)
		assert.NotEmpty(t, received.Sender) // Sender should be set
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestHub_OnMessage_InvalidJSON(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	messageReceived := false
	hub.OnMessage(func(c *Client, m *Message) {
		messageReceived = true
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Send invalid JSON - should be silently ignored
	err = conn.WriteMessage(ws.TextMessage, []byte("not valid json"))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	assert.False(t, messageReceived, "invalid JSON should not trigger message handler")
}

func TestPingPong_Timeout(t *testing.T) {
	// Use very short timeouts to test ping/pong behavior
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    50 * time.Millisecond,
		PongWait:        100 * time.Millisecond,
		WriteWait:       50 * time.Millisecond,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Create a custom dialer that doesn't respond to pings
	dialer := ws.Dialer{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Wait for ping timeout
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())

	// Set a handler that does nothing with pings (client won't respond)
	conn.SetPingHandler(func(appData string) error {
		// Don't respond to ping - simulate timeout
		return nil
	})

	// Wait for the pong wait timeout (read deadline)
	time.Sleep(200 * time.Millisecond)

	// The client should eventually be disconnected due to read deadline
	_ = conn.Close()
}

func TestWritePump_ChannelClose(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Get the client
	hub.clientsMu.RLock()
	var client *Client
	for _, c := range hub.clients {
		client = c
		break
	}
	hub.clientsMu.RUnlock()

	require.NotNil(t, client)

	// Close the client - this should trigger writePump to exit gracefully
	// by closing the send channel
	err = client.Close()
	assert.NoError(t, err)

	// Verify client is closed
	client.mu.Lock()
	assert.True(t, client.closed)
	client.mu.Unlock()
}

func TestReadPump_ConnectionClose(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())

	// Close the connection from client side
	err = conn.WriteMessage(ws.CloseMessage, ws.FormatCloseMessage(ws.CloseNormalClosure, ""))
	require.NoError(t, err)
	_ = conn.Close()

	// Wait for the hub to process the unregister
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, hub.ClientCount())
}

func TestReadPump_AbnormalClosure(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())

	// Force close without proper close message (abnormal closure)
	conn.UnderlyingConn().Close()

	// Wait for the hub to process the unregister
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, hub.ClientCount())
}

func TestBroadcast_MarshalError(t *testing.T) {
	hub := NewHub(nil)
	defer hub.Close()

	// Create a message with invalid JSON in RawMessage - this will fail to marshal
	msg := &Message{
		Type: "test",
		Data: json.RawMessage(`invalid json that looks valid`),
	}

	// This should fail because RawMessage validates JSON on marshal
	err := hub.Broadcast(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal message")
}

func TestSendToRoom_MarshalError(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Get client and join room
	hub.clientsMu.RLock()
	var clientID string
	for id := range hub.clients {
		clientID = id
		break
	}
	hub.clientsMu.RUnlock()

	err = hub.JoinRoom(clientID, "test-room")
	require.NoError(t, err)

	// Create a message with invalid JSON - should fail to marshal
	msg := &Message{
		Type: "test",
		Data: json.RawMessage(`not valid json`),
	}

	err = hub.SendToRoom("test-room", msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal message")
}

func TestSendToRoom_ClientSendError(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Get client and join room
	hub.clientsMu.RLock()
	var clientID string
	var client *Client
	for id, c := range hub.clients {
		clientID = id
		client = c
		break
	}
	hub.clientsMu.RUnlock()

	err = hub.JoinRoom(clientID, "test-room")
	require.NoError(t, err)

	// Fill the client's send buffer to cause send errors
	for i := 0; i < 300; i++ {
		_ = client.Send([]byte("fill"))
	}

	// Send to room - should not return error even if individual sends fail
	// (SendToRoom ignores individual client send errors)
	msg := &Message{Type: "room-msg", Data: json.RawMessage(`{"test":true}`)}
	err = hub.SendToRoom("test-room", msg)
	assert.NoError(t, err)
}

func TestHub_LeaveRoom_ClientNotInRoom(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Get client ID
	hub.clientsMu.RLock()
	var clientID string
	for id := range hub.clients {
		clientID = id
		break
	}
	hub.clientsMu.RUnlock()

	// Create room with a different client by joining then leaving
	err = hub.JoinRoom(clientID, "empty-room")
	require.NoError(t, err)

	// Leave the room - should work
	err = hub.LeaveRoom(clientID, "empty-room")
	assert.NoError(t, err)

	// Leave again - room exists but client not in it
	// First rejoin to create the room again
	err = hub.JoinRoom(clientID, "another-room")
	require.NoError(t, err)

	// Leave a room where client was never added (using a fake client ID)
	err = hub.LeaveRoom("nonexistent-client", "another-room")
	assert.NoError(t, err) // Should not error, just no-op for nonexistent client in room
}

func TestClient_ID(t *testing.T) {
	client := &Client{
		id:    "test-id-123",
		send:  make(chan []byte, 10),
		rooms: make(map[string]bool),
	}

	assert.Equal(t, "test-id-123", client.ID())
}

func TestHub_UnregisterRemovesFromRooms(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Get client and join multiple rooms
	hub.clientsMu.RLock()
	var clientID string
	for id := range hub.clients {
		clientID = id
		break
	}
	hub.clientsMu.RUnlock()

	err = hub.JoinRoom(clientID, "room1")
	require.NoError(t, err)
	err = hub.JoinRoom(clientID, "room2")
	require.NoError(t, err)

	assert.Equal(t, 1, hub.RoomClientCount("room1"))
	assert.Equal(t, 1, hub.RoomClientCount("room2"))

	// Disconnect client
	_ = conn.Close()

	// Wait for unregister to process
	time.Sleep(100 * time.Millisecond)

	// Client should be removed from all rooms
	assert.Equal(t, 0, hub.ClientCount())
	assert.Equal(t, 0, hub.RoomClientCount("room1"))
	assert.Equal(t, 0, hub.RoomClientCount("room2"))
}

func TestPongHandler_ExtendsDeadline(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    50 * time.Millisecond,
		PongWait:        200 * time.Millisecond,
		WriteWait:       50 * time.Millisecond,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	dialer := ws.Dialer{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Set up pong handler that responds with pong
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteControl(ws.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// Start a goroutine to read and respond to pings
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Wait long enough for multiple ping/pong cycles
	time.Sleep(150 * time.Millisecond)

	// Client should still be connected because pongs are keeping connection alive
	assert.Equal(t, 1, hub.ClientCount())
}

func TestHub_Close_StopsReadPump(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())

	// Close the hub - this should trigger ctx.Done() in read/write pumps
	hub.Close()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// All clients should be disconnected
	assert.Equal(t, 0, hub.ClientCount())
}

func TestBroadcast_ToMultipleClients_SomeFailures(t *testing.T) {
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})
	defer hub.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect multiple clients
	conn1, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn1.Close() }()

	conn2, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn2.Close() }()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, hub.ClientCount())

	// Fill one client's buffer to cause send failure
	hub.clientsMu.RLock()
	var firstClient *Client
	for _, c := range hub.clients {
		firstClient = c
		break
	}
	hub.clientsMu.RUnlock()

	// Fill buffer
	for i := 0; i < 300; i++ {
		_ = firstClient.Send([]byte("fill"))
	}

	// Broadcast should succeed even if some sends fail
	msg := &Message{Type: "broadcast", Data: json.RawMessage(`{"test":true}`)}
	err = hub.Broadcast(msg)
	assert.NoError(t, err)
}

func TestReadPump_ContextDone_ImmediateReturn(t *testing.T) {
	// Test the ctx.Done() path in readPump (lines 145-146)
	// This path is hit when the context is cancelled during the select check
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    1 * time.Hour,
		PongWait:        1 * time.Hour,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = hub.ServeWS(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())

	// Rapidly close the hub multiple times in a loop to maximize the chance
	// of hitting the ctx.Done() case in the select statement
	// The race between ReadMessage blocking and select checking ctx.Done()
	// is timing-dependent, so we run multiple iterations
	for i := 0; i < 10; i++ {
		hub := NewHub(&Config{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			PingInterval:    1 * time.Millisecond,
			PongWait:        1 * time.Millisecond,
			WriteWait:       1 * time.Millisecond,
			MaxMessageSize:  512 * 1024,
			AllowedOrigins:  []string{"*"},
		})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = hub.ServeWS(w, r)
		}))

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		testConn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			server.Close()
			continue
		}

		// Very short delay then close - try to hit the ctx.Done() case
		time.Sleep(100 * time.Microsecond)
		hub.Close()
		_ = testConn.Close()
		server.Close()
	}

	_ = conn.Close()
	hub.Close()
}

func TestReadPump_ContextCancelledBeforeStart(t *testing.T) {
	// Create a hub and server
	hub := NewHub(&Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    30 * time.Second,
		PongWait:        60 * time.Second,
		WriteWait:       10 * time.Second,
		MaxMessageSize:  512 * 1024,
		AllowedOrigins:  []string{"*"},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := hub.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Create a client manually
		client := &Client{
			id:    "test-client",
			conn:  conn,
			send:  make(chan []byte, 256),
			hub:   hub,
			rooms: make(map[string]bool),
		}

		// Register the client
		hub.register <- client

		// Start writePump normally
		go client.writePump(hub.config, hub.ctx)

		// Create a pre-cancelled context for readPump
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// This should return immediately due to ctx.Err() check
		client.readPump(hub.config, cancelledCtx)
	}))
	defer server.Close()
	defer hub.Close()

	// Connect to trigger the handler
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Skipf("could not connect: %v", err)
	}
	defer conn.Close()

	// Give time for the readPump to exit
	time.Sleep(100 * time.Millisecond)
}
