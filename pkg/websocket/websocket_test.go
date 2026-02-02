package websocket

import (
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
