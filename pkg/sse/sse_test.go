package sse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvent_Format(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected string
	}{
		{
			name: "data only",
			event: Event{
				Data: []byte("hello"),
			},
			expected: "data: hello\n\n",
		},
		{
			name: "with type",
			event: Event{
				Type: "message",
				Data: []byte("hello"),
			},
			expected: "event: message\ndata: hello\n\n",
		},
		{
			name: "with id and type",
			event: Event{
				ID:   "123",
				Type: "update",
				Data: []byte(`{"key":"value"}`),
			},
			expected: "id: 123\nevent: update\ndata: {\"key\":\"value\"}\n\n",
		},
		{
			name: "with retry",
			event: Event{
				Type:  "message",
				Data:  []byte("reconnect"),
				Retry: 5000,
			},
			expected: "event: message\nretry: 5000\ndata: reconnect\n\n",
		},
		{
			name: "all fields",
			event: Event{
				ID:    "42",
				Type:  "notification",
				Data:  []byte("payload"),
				Retry: 3000,
			},
			expected: "id: 42\nevent: notification\nretry: 3000\ndata: payload\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.Format()
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, 100, config.BufferSize)
	assert.Equal(t, 30*time.Second, config.HeartbeatInterval)
	assert.Equal(t, 1000, config.MaxClients)
}

func TestNewBroker(t *testing.T) {
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
				BufferSize:        50,
				HeartbeatInterval: 10 * time.Second,
				MaxClients:        500,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := NewBroker(tt.config)
			require.NotNil(t, broker)
			assert.Equal(t, 0, broker.ClientCount())
			broker.Close()
		})
	}
}

func TestBroker_Broadcast(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour, // disable heartbeat interference
		MaxClients:        100,
	})
	defer broker.Close()

	// Create mock clients directly
	client1 := &Client{
		ID:        "c1",
		Channel:   make(chan []byte, 10),
		CreatedAt: time.Now(),
	}
	client2 := &Client{
		ID:        "c2",
		Channel:   make(chan []byte, 10),
		CreatedAt: time.Now(),
	}

	broker.clientsMu.Lock()
	broker.clients["c1"] = client1
	broker.clients["c2"] = client2
	broker.clientCount.Add(2)
	broker.clientsMu.Unlock()

	event := &Event{
		Type: "test",
		Data: []byte("broadcast data"),
	}
	broker.Broadcast(event)

	expected := event.Format()

	select {
	case data := <-client1.Channel:
		assert.Equal(t, string(expected), string(data))
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client1 data")
	}

	select {
	case data := <-client2.Channel:
		assert.Equal(t, string(expected), string(data))
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client2 data")
	}
}

func TestBroker_SendTo(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	tests := []struct {
		name      string
		clientID  string
		setup     func()
		expectErr bool
	}{
		{
			name:     "send to existing client",
			clientID: "c1",
			setup: func() {
				broker.clientsMu.Lock()
				broker.clients["c1"] = &Client{
					ID:      "c1",
					Channel: make(chan []byte, 10),
				}
				broker.clientCount.Add(1)
				broker.clientsMu.Unlock()
			},
			expectErr: false,
		},
		{
			name:      "send to non-existent client",
			clientID:  "missing",
			setup:     func() {},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			event := &Event{Type: "test", Data: []byte("hello")}
			err := broker.SendTo(tt.clientID, event)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBroker_SendTo_FullChannel(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        1,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	client := &Client{
		ID:      "full",
		Channel: make(chan []byte, 1),
	}

	broker.clientsMu.Lock()
	broker.clients["full"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	// Fill the channel
	event := &Event{Type: "test", Data: []byte("first")}
	err := broker.SendTo("full", event)
	require.NoError(t, err)

	// Now channel is full
	err = broker.SendTo("full", &Event{Type: "test", Data: []byte("second")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client channel full")
}

func TestBroker_RemoveClient(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	client := &Client{
		ID:      "remove-me",
		Channel: make(chan []byte, 10),
	}

	broker.clientsMu.Lock()
	broker.clients["remove-me"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	assert.Equal(t, 1, broker.ClientCount())

	broker.RemoveClient("remove-me")
	assert.Equal(t, 0, broker.ClientCount())

	// Removing again is a no-op
	broker.RemoveClient("remove-me")
	assert.Equal(t, 0, broker.ClientCount())
}

func TestBroker_Close(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 100 * time.Millisecond,
		MaxClients:        100,
	})

	client := &Client{
		ID:      "closeable",
		Channel: make(chan []byte, 10),
	}

	broker.clientsMu.Lock()
	broker.clients["closeable"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	broker.Close()
	assert.Equal(t, 0, broker.ClientCount())
}

func TestBroker_ClientCount(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	assert.Equal(t, 0, broker.ClientCount())

	for i := 0; i < 5; i++ {
		id := "c" + string(rune('0'+i))
		broker.clientsMu.Lock()
		broker.clients[id] = &Client{
			ID:      id,
			Channel: make(chan []byte, 10),
		}
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	assert.Equal(t, 5, broker.ClientCount())
}
