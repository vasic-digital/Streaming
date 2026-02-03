package sse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Event Tests
// =============================================================================

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
		{
			name: "empty data",
			event: Event{
				Type: "ping",
				Data: []byte(""),
			},
			expected: "event: ping\ndata: \n\n",
		},
		{
			name: "id only",
			event: Event{
				ID:   "event-1",
				Data: []byte("data"),
			},
			expected: "id: event-1\ndata: data\n\n",
		},
		{
			name: "zero retry",
			event: Event{
				Type:  "test",
				Data:  []byte("data"),
				Retry: 0,
			},
			expected: "event: test\ndata: data\n\n",
		},
		{
			name: "negative retry is ignored",
			event: Event{
				Type:  "test",
				Data:  []byte("data"),
				Retry: -100,
			},
			expected: "event: test\ndata: data\n\n",
		},
		{
			name: "large retry value",
			event: Event{
				Type:  "test",
				Data:  []byte("data"),
				Retry: 60000,
			},
			expected: "event: test\nretry: 60000\ndata: data\n\n",
		},
		{
			name: "binary-like data",
			event: Event{
				Data: []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f}, // "Hello"
			},
			expected: "data: Hello\n\n",
		},
		{
			name: "multiline data",
			event: Event{
				Data: []byte("line1\nline2\nline3"),
			},
			expected: "data: line1\nline2\nline3\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.Format()
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestEvent_Format_Concurrent(t *testing.T) {
	event := &Event{
		ID:    "concurrent-id",
		Type:  "concurrent-event",
		Data:  []byte("concurrent-data"),
		Retry: 1000,
	}

	var wg sync.WaitGroup
	expected := event.Format()

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := event.Format()
			assert.Equal(t, string(expected), string(result))
		}()
	}

	wg.Wait()
}

// =============================================================================
// Config Tests
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, 100, config.BufferSize)
	assert.Equal(t, 30*time.Second, config.HeartbeatInterval)
	assert.Equal(t, 1000, config.MaxClients)
}

func TestConfig_Fields(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "minimal config",
			config: Config{
				BufferSize:        1,
				HeartbeatInterval: 1 * time.Second,
				MaxClients:        1,
			},
		},
		{
			name: "large config",
			config: Config{
				BufferSize:        10000,
				HeartbeatInterval: 1 * time.Hour,
				MaxClients:        100000,
			},
		},
		{
			name: "zero heartbeat",
			config: Config{
				BufferSize:        100,
				HeartbeatInterval: 0,
				MaxClients:        100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.GreaterOrEqual(t, tt.config.BufferSize, 0)
			assert.GreaterOrEqual(t, tt.config.MaxClients, 0)
		})
	}
}

// =============================================================================
// Broker Tests
// =============================================================================

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
		{
			name: "zero buffer size",
			config: &Config{
				BufferSize:        0,
				HeartbeatInterval: 1 * time.Hour,
				MaxClients:        100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := NewBroker(tt.config)
			require.NotNil(t, broker)
			assert.Equal(t, 0, broker.ClientCount())
			assert.NotNil(t, broker.clients)
			assert.NotNil(t, broker.ctx)
			assert.NotNil(t, broker.cancel)
			broker.Close()
		})
	}
}

func TestNewBroker_ConfigApplied(t *testing.T) {
	config := &Config{
		BufferSize:        42,
		HeartbeatInterval: 5 * time.Second,
		MaxClients:        123,
	}

	broker := NewBroker(config)
	defer broker.Close()

	assert.Equal(t, 42, broker.config.BufferSize)
	assert.Equal(t, 5*time.Second, broker.config.HeartbeatInterval)
	assert.Equal(t, 123, broker.config.MaxClients)
}

func TestBroker_ClientCount(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	assert.Equal(t, 0, broker.ClientCount())

	// Add clients manually
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("client-%d", i)
		broker.clientsMu.Lock()
		broker.clients[id] = &Client{
			ID:        id,
			Channel:   make(chan []byte, 10),
			CreatedAt: time.Now(),
		}
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	assert.Equal(t, 5, broker.ClientCount())
}

func TestBroker_ClientCount_Concurrent(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        1000,
	})
	defer broker.Close()

	var wg sync.WaitGroup

	// Add clients concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("client-%d", i)
			broker.clientsMu.Lock()
			broker.clients[id] = &Client{
				ID:        id,
				Channel:   make(chan []byte, 10),
				CreatedAt: time.Now(),
			}
			broker.clientCount.Add(1)
			broker.clientsMu.Unlock()
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 100, broker.ClientCount())
}

// =============================================================================
// Broadcast Tests
// =============================================================================

func TestBroker_Broadcast(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
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

func TestBroker_Broadcast_NoClients(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	// Should not panic with no clients
	assert.NotPanics(t, func() {
		broker.Broadcast(&Event{Type: "test", Data: []byte("data")})
	})
}

func TestBroker_Broadcast_FullChannel(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        1,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	client := &Client{
		ID:        "full-client",
		Channel:   make(chan []byte, 1),
		CreatedAt: time.Now(),
	}

	broker.clientsMu.Lock()
	broker.clients["full-client"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	// Fill the channel
	broker.Broadcast(&Event{Type: "test", Data: []byte("first")})

	// Second broadcast should skip (not block)
	done := make(chan bool)
	go func() {
		broker.Broadcast(&Event{Type: "test", Data: []byte("second")})
		done <- true
	}()

	select {
	case <-done:
		// Success - broadcast didn't block
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on full channel")
	}
}

func TestBroker_Broadcast_Concurrent(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        100,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	// Add some clients
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("client-%d", i)
		broker.clientsMu.Lock()
		broker.clients[id] = &Client{
			ID:        id,
			Channel:   make(chan []byte, 100),
			CreatedAt: time.Now(),
		}
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			broker.Broadcast(&Event{
				Type: "concurrent",
				Data: []byte(fmt.Sprintf("message-%d", i)),
			})
		}(i)
	}

	wg.Wait()
	// No panics or deadlocks
}

// =============================================================================
// SendTo Tests
// =============================================================================

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
		errMsg    string
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
			errMsg:    "client not found: missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			event := &Event{Type: "test", Data: []byte("hello")}
			err := broker.SendTo(tt.clientID, event)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
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

func TestBroker_SendTo_VerifyData(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	client := &Client{
		ID:      "receiver",
		Channel: make(chan []byte, 10),
	}

	broker.clientsMu.Lock()
	broker.clients["receiver"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	event := &Event{
		ID:   "msg-1",
		Type: "notification",
		Data: []byte(`{"action":"update"}`),
	}

	err := broker.SendTo("receiver", event)
	require.NoError(t, err)

	select {
	case data := <-client.Channel:
		assert.Equal(t, string(event.Format()), string(data))
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for data")
	}
}

// =============================================================================
// RemoveClient Tests
// =============================================================================

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

	// Verify client is removed from map
	broker.clientsMu.RLock()
	_, exists := broker.clients["remove-me"]
	broker.clientsMu.RUnlock()
	assert.False(t, exists)
}

func TestBroker_RemoveClient_NonExistent(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	// Should not panic
	assert.NotPanics(t, func() {
		broker.RemoveClient("non-existent")
	})
	assert.Equal(t, 0, broker.ClientCount())
}

func TestBroker_RemoveClient_Idempotent(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	client := &Client{
		ID:      "remove-twice",
		Channel: make(chan []byte, 10),
	}

	broker.clientsMu.Lock()
	broker.clients["remove-twice"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	broker.RemoveClient("remove-twice")
	assert.Equal(t, 0, broker.ClientCount())

	// Removing again should be a no-op
	broker.RemoveClient("remove-twice")
	assert.Equal(t, 0, broker.ClientCount())
}

func TestBroker_RemoveClient_Concurrent(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        1000,
	})
	defer broker.Close()

	// Add clients
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("client-%d", i)
		broker.clientsMu.Lock()
		broker.clients[id] = &Client{
			ID:      id,
			Channel: make(chan []byte, 10),
		}
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	// Remove concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			broker.RemoveClient(fmt.Sprintf("client-%d", i))
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 0, broker.ClientCount())
}

// =============================================================================
// Close Tests
// =============================================================================

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

func TestBroker_Close_MultipleClients(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("client-%d", i)
		broker.clientsMu.Lock()
		broker.clients[id] = &Client{
			ID:      id,
			Channel: make(chan []byte, 10),
		}
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	assert.Equal(t, 10, broker.ClientCount())

	broker.Close()
	assert.Equal(t, 0, broker.ClientCount())

	// Verify all clients are removed
	broker.clientsMu.RLock()
	assert.Empty(t, broker.clients)
	broker.clientsMu.RUnlock()
}

func TestBroker_Close_Idempotent(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})

	// Close multiple times should not panic
	assert.NotPanics(t, func() {
		broker.Close()
		broker.Close()
	})
}

// =============================================================================
// AddClient Tests
// =============================================================================

// mockResponseWriter implements http.ResponseWriter without Flusher.
type mockResponseWriter struct {
	mu      sync.Mutex
	headers http.Header
	body    []byte
	status  int
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{
		headers: make(http.Header),
	}
}

func (m *mockResponseWriter) Header() http.Header {
	return m.headers
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = statusCode
}

func (m *mockResponseWriter) Body() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.body
}

// mockFlusherResponseWriter implements http.ResponseWriter with Flusher.
type mockFlusherResponseWriter struct {
	mockResponseWriter
	flushCount int
}

func newMockFlusherResponseWriter() *mockFlusherResponseWriter {
	return &mockFlusherResponseWriter{
		mockResponseWriter: mockResponseWriter{
			headers: make(http.Header),
		},
	}
}

func (m *mockFlusherResponseWriter) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushCount++
}

func (m *mockFlusherResponseWriter) FlushCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushCount
}

func TestBroker_AddClient_NoFlusher(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockResponseWriter()
	r := httptest.NewRequest(http.MethodGet, "/events", nil)

	client, err := broker.AddClient(w, r)
	assert.Nil(t, client)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "streaming unsupported")
}

func TestBroker_AddClient_MaxClientsReached(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        2,
	})
	defer broker.Close()

	// Fill up to max
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("client-%d", i)
		broker.clientsMu.Lock()
		broker.clients[id] = &Client{
			ID:      id,
			Channel: make(chan []byte, 10),
		}
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	w := newMockFlusherResponseWriter()
	r := httptest.NewRequest(http.MethodGet, "/events", nil)

	client, err := broker.AddClient(w, r)
	assert.Nil(t, client)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum client limit reached")
}

func TestBroker_AddClient_Success(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.NotEmpty(t, client.ID)
	assert.NotNil(t, client.Channel)
	assert.False(t, client.CreatedAt.IsZero())
	assert.Equal(t, 1, broker.ClientCount())

	// Check headers are set
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", w.Header().Get("Connection"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
}

func TestBroker_AddClient_WithClientID(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("X-Client-ID", "custom-client-123")

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, "custom-client-123", client.ID)
}

func TestBroker_AddClient_WithLastEventID(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("Last-Event-ID", "event-42")

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, "event-42", client.LastEventID)
}

func TestBroker_AddClient_ZeroMaxClients(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        0, // Zero means unlimited
	})
	defer broker.Close()

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestBroker_AddClient_DisconnectRemovesClient(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("X-Client-ID", "disconnect-test")

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, 1, broker.ClientCount())

	// Cancel context to simulate disconnect
	cancel()

	// Wait for goroutine to process
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 0, broker.ClientCount())
}

func TestBroker_AddClient_StreamingGoroutine(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("X-Client-ID", "stream-test")

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Send data through the channel and verify it's written
	event := &Event{Type: "test", Data: []byte("hello")}
	client.Channel <- event.Format()

	// Wait for the goroutine to process
	time.Sleep(50 * time.Millisecond)

	// Verify the data was written (using thread-safe accessor)
	assert.Contains(t, string(w.Body()), "hello")
	assert.Greater(t, w.FlushCount(), 0)
}

func TestBroker_AddClient_StreamingStopsOnBrokerClose(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("X-Client-ID", "broker-close-test")

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Send an event
	event := &Event{Type: "test", Data: []byte("before close")}
	client.Channel <- event.Format()

	// Wait for the goroutine to process
	time.Sleep(50 * time.Millisecond)

	// Close broker - this should stop the streaming goroutine
	broker.Close()

	// Wait for cleanup
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, broker.ClientCount())
}

func TestBroker_AddClient_StreamingStopsOnChannelClose(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockFlusherResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("X-Client-ID", "channel-close-test")

	client, err := broker.AddClient(w, r)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Send an event
	event := &Event{Type: "test", Data: []byte("before channel close")}
	client.Channel <- event.Format()

	// Wait for the goroutine to process
	time.Sleep(50 * time.Millisecond)

	// Remove client - this closes the channel
	broker.RemoveClient("channel-close-test")

	// Wait for cleanup
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, broker.ClientCount())
}

// =============================================================================
// ServeHTTP Tests
// =============================================================================

func TestBroker_ServeHTTP_NoFlusher(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	w := newMockResponseWriter()
	r := httptest.NewRequest(http.MethodGet, "/events", nil)

	broker.ServeHTTP(w, r)

	assert.Equal(t, http.StatusServiceUnavailable, w.status)
	assert.Contains(t, string(w.body), "streaming unsupported")
}

func TestBroker_ServeHTTP_MaxClients(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        1,
	})
	defer broker.Close()

	// Fill up
	broker.clientsMu.Lock()
	broker.clients["existing"] = &Client{
		ID:      "existing",
		Channel: make(chan []byte, 10),
	}
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	recorder := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/events", nil)

	broker.ServeHTTP(recorder, r)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "maximum client limit")
}

func TestBroker_ServeHTTP_Success(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)

	done := make(chan bool)
	go func() {
		broker.ServeHTTP(recorder, r)
		done <- true
	}()

	select {
	case <-done:
		// Success - ServeHTTP returned after context cancellation
	case <-time.After(time.Second):
		t.Fatal("ServeHTTP did not return after context cancellation")
	}
}

// =============================================================================
// Heartbeat Tests
// =============================================================================

func TestBroker_Heartbeat(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 50 * time.Millisecond,
		MaxClients:        100,
	})
	defer broker.Close()

	client := &Client{
		ID:      "heartbeat-client",
		Channel: make(chan []byte, 10),
	}

	broker.clientsMu.Lock()
	broker.clients["heartbeat-client"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	// Wait for heartbeat
	select {
	case data := <-client.Channel:
		assert.Contains(t, string(data), "heartbeat")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("did not receive heartbeat")
	}
}

func TestBroker_Heartbeat_StopsOnClose(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 10 * time.Millisecond,
		MaxClients:        100,
	})

	client := &Client{
		ID:      "heartbeat-stop",
		Channel: make(chan []byte, 100),
	}

	broker.clientsMu.Lock()
	broker.clients["heartbeat-stop"] = client
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	// Wait for at least one heartbeat
	select {
	case <-client.Channel:
		// Received heartbeat
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive heartbeat before close")
	}

	// Close broker - this will also close client channels
	broker.Close()

	// Verify broker closed properly
	assert.Equal(t, 0, broker.ClientCount())
}

// =============================================================================
// Client Tests
// =============================================================================

func TestClient_Fields(t *testing.T) {
	now := time.Now()
	client := &Client{
		ID:          "test-client",
		Channel:     make(chan []byte, 10),
		LastEventID: "last-123",
		CreatedAt:   now,
	}

	assert.Equal(t, "test-client", client.ID)
	assert.NotNil(t, client.Channel)
	assert.Equal(t, "last-123", client.LastEventID)
	assert.Equal(t, now, client.CreatedAt)
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestBroker_Integration_BroadcastToMultipleClients(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        100,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	// Add multiple clients
	clients := make([]*Client, 5)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("client-%d", i)
		clients[i] = &Client{
			ID:        id,
			Channel:   make(chan []byte, 100),
			CreatedAt: time.Now(),
		}
		broker.clientsMu.Lock()
		broker.clients[id] = clients[i]
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	// Broadcast multiple events
	for i := 0; i < 10; i++ {
		broker.Broadcast(&Event{
			ID:   fmt.Sprintf("event-%d", i),
			Type: "message",
			Data: []byte(fmt.Sprintf("message %d", i)),
		})
	}

	// Verify all clients received all messages
	for i, client := range clients {
		received := 0
		for j := 0; j < 10; j++ {
			select {
			case <-client.Channel:
				received++
			case <-time.After(100 * time.Millisecond):
				// Timeout
			}
		}
		assert.Equal(t, 10, received, "client %d did not receive all messages", i)
	}
}

func TestBroker_Integration_ConcurrentOperations(t *testing.T) {
	broker := NewBroker(&Config{
		BufferSize:        100,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        1000,
	})
	defer broker.Close()

	var wg sync.WaitGroup

	// Phase 1: Add clients
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("add-client-%d", i)
			broker.clientsMu.Lock()
			broker.clients[id] = &Client{
				ID:      id,
				Channel: make(chan []byte, 100),
			}
			broker.clientCount.Add(1)
			broker.clientsMu.Unlock()
		}(i)
	}
	wg.Wait()

	// Phase 2: Concurrent Broadcast and SendTo (no removes yet to avoid race)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			broker.Broadcast(&Event{
				Type: "concurrent",
				Data: []byte(fmt.Sprintf("broadcast-%d", i)),
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = broker.SendTo(fmt.Sprintf("add-client-%d", i), &Event{
				Type: "direct",
				Data: []byte(fmt.Sprintf("direct-%d", i)),
			})
		}(i)
	}
	wg.Wait()

	// Phase 3: Remove clients concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			broker.RemoveClient(fmt.Sprintf("add-client-%d", i))
		}(i)
	}
	wg.Wait()

	// No panics or deadlocks
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkEvent_Format(b *testing.B) {
	event := &Event{
		ID:    "bench-id",
		Type:  "bench-type",
		Data:  []byte("benchmark data payload"),
		Retry: 3000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.Format()
	}
}

func BenchmarkBroker_Broadcast(b *testing.B) {
	broker := NewBroker(&Config{
		BufferSize:        1000,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	// Add clients
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("client-%d", i)
		broker.clientsMu.Lock()
		broker.clients[id] = &Client{
			ID:      id,
			Channel: make(chan []byte, 1000),
		}
		broker.clientCount.Add(1)
		broker.clientsMu.Unlock()
	}

	event := &Event{
		Type: "bench",
		Data: []byte("benchmark payload"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.Broadcast(event)
	}
}

func BenchmarkBroker_SendTo(b *testing.B) {
	broker := NewBroker(&Config{
		BufferSize:        10000,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        100,
	})
	defer broker.Close()

	broker.clientsMu.Lock()
	broker.clients["target"] = &Client{
		ID:      "target",
		Channel: make(chan []byte, 10000),
	}
	broker.clientCount.Add(1)
	broker.clientsMu.Unlock()

	event := &Event{
		Type: "bench",
		Data: []byte("benchmark payload"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = broker.SendTo("target", event)
	}
}

func BenchmarkBroker_ClientCount(b *testing.B) {
	broker := NewBroker(&Config{
		BufferSize:        10,
		HeartbeatInterval: 1 * time.Hour,
		MaxClients:        1000,
	})
	defer broker.Close()

	for i := 0; i < 100; i++ {
		broker.clientCount.Add(1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = broker.ClientCount()
	}
}
