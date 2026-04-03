package sse_test

import (
	"testing"
	"time"

	"digital.vasic.streaming/pkg/sse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Event Format Edge Cases ---

func TestEvent_Format_ZeroLengthData(t *testing.T) {
	t.Parallel()

	e := &sse.Event{
		Data: []byte{},
	}
	formatted := e.Format()
	assert.Equal(t, "data: \n\n", string(formatted))
}

func TestEvent_Format_NilData(t *testing.T) {
	t.Parallel()

	e := &sse.Event{
		Data: nil,
	}
	formatted := e.Format()
	assert.Equal(t, "data: \n\n", string(formatted))
}

func TestEvent_Format_AllFieldsSet(t *testing.T) {
	t.Parallel()

	e := &sse.Event{
		ID:    "evt-123",
		Type:  "message",
		Data:  []byte("hello world"),
		Retry: 5000,
	}
	formatted := string(e.Format())

	assert.Contains(t, formatted, "id: evt-123\n")
	assert.Contains(t, formatted, "event: message\n")
	assert.Contains(t, formatted, "retry: 5000\n")
	assert.Contains(t, formatted, "data: hello world\n\n")
}

func TestEvent_Format_OnlyData(t *testing.T) {
	t.Parallel()

	e := &sse.Event{
		Data: []byte("just data"),
	}
	formatted := string(e.Format())

	assert.NotContains(t, formatted, "id:")
	assert.NotContains(t, formatted, "event:")
	assert.NotContains(t, formatted, "retry:")
	assert.Equal(t, "data: just data\n\n", formatted)
}

func TestEvent_Format_LargeData(t *testing.T) {
	t.Parallel()

	largePayload := make([]byte, 1024*1024) // 1MB
	for i := range largePayload {
		largePayload[i] = 'x'
	}

	e := &sse.Event{
		Data: largePayload,
	}
	formatted := e.Format()
	assert.True(t, len(formatted) > 1024*1024)
}

func TestEvent_Format_UnicodeData(t *testing.T) {
	t.Parallel()

	e := &sse.Event{
		Data: []byte("\u4e2d\u6587\u6d88\u606f \U0001f600"),
	}
	formatted := string(e.Format())
	assert.Contains(t, formatted, "\u4e2d\u6587\u6d88\u606f \U0001f600")
}

func TestEvent_Format_ZeroRetry(t *testing.T) {
	t.Parallel()

	e := &sse.Event{
		Data:  []byte("test"),
		Retry: 0, // Should not include retry field
	}
	formatted := string(e.Format())
	assert.NotContains(t, formatted, "retry:")
}

func TestEvent_Format_NegativeRetry(t *testing.T) {
	t.Parallel()

	e := &sse.Event{
		Data:  []byte("test"),
		Retry: -1, // Negative should not include retry field
	}
	formatted := string(e.Format())
	assert.NotContains(t, formatted, "retry:")
}

// --- Broker Edge Cases ---

func TestBroker_NilConfig_UsesDefaults(t *testing.T) {
	t.Parallel()

	b := sse.NewBroker(nil)
	require.NotNil(t, b)
	defer b.Close()

	assert.Equal(t, 0, b.ClientCount())
}

func TestBroker_Broadcast_NoClients(t *testing.T) {
	t.Parallel()

	b := sse.NewBroker(nil)
	defer b.Close()

	// Should not panic with no clients
	b.Broadcast(&sse.Event{
		Type: "test",
		Data: []byte("hello"),
	})
}

func TestBroker_SendTo_NonexistentClient(t *testing.T) {
	t.Parallel()

	b := sse.NewBroker(nil)
	defer b.Close()

	err := b.SendTo("nonexistent-client", &sse.Event{
		Data: []byte("hello"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client not found")
}

func TestBroker_RemoveClient_Nonexistent(t *testing.T) {
	t.Parallel()

	b := sse.NewBroker(nil)
	defer b.Close()

	// Should not panic
	b.RemoveClient("does-not-exist")
	assert.Equal(t, 0, b.ClientCount())
}

func TestBroker_DoubleClose(t *testing.T) {
	t.Parallel()

	b := sse.NewBroker(nil)
	b.Close()

	// Second close should not panic
	b.Close()
}

func TestBroker_CloseWithNoClients(t *testing.T) {
	t.Parallel()

	b := sse.NewBroker(nil)
	b.Close()
	assert.Equal(t, 0, b.ClientCount())
}

// --- Broker Config Edge Cases ---

func TestBroker_ZeroBufferSize(t *testing.T) {
	t.Parallel()

	cfg := &sse.Config{
		BufferSize:        0,
		HeartbeatInterval: time.Second,
		MaxClients:        10,
	}
	b := sse.NewBroker(cfg)
	require.NotNil(t, b)
	defer b.Close()
}

func TestBroker_ZeroMaxClients(t *testing.T) {
	t.Parallel()

	cfg := &sse.Config{
		BufferSize:        100,
		HeartbeatInterval: time.Second,
		MaxClients:        0, // No limit
	}
	b := sse.NewBroker(cfg)
	require.NotNil(t, b)
	defer b.Close()
}

func TestBroker_VeryShortHeartbeat(t *testing.T) {
	t.Parallel()

	cfg := &sse.Config{
		BufferSize:        100,
		HeartbeatInterval: time.Millisecond,
		MaxClients:        10,
	}
	b := sse.NewBroker(cfg)
	require.NotNil(t, b)

	// Let it run a few heartbeat cycles
	time.Sleep(10 * time.Millisecond)
	b.Close()
}

// --- DefaultConfig Values ---

func TestDefaultConfig_Values(t *testing.T) {
	t.Parallel()

	cfg := sse.DefaultConfig()
	assert.Equal(t, 100, cfg.BufferSize)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 1000, cfg.MaxClients)
}

// --- Client Struct ---

func TestClient_Fields(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 10)
	now := time.Now()

	c := &sse.Client{
		ID:          "test-client",
		Channel:     ch,
		LastEventID: "evt-5",
		CreatedAt:   now,
	}

	assert.Equal(t, "test-client", c.ID)
	assert.Equal(t, "evt-5", c.LastEventID)
	assert.Equal(t, now, c.CreatedAt)
}

func TestClient_EmptyID(t *testing.T) {
	t.Parallel()

	c := &sse.Client{
		ID: "",
	}
	assert.Empty(t, c.ID)
}
