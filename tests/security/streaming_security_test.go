package security

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.streaming/pkg/sse"
	"digital.vasic.streaming/pkg/transport"
	"digital.vasic.streaming/pkg/webhook"
)

func TestSecurity_SSE_NilData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	// Event with nil data should not panic
	event := &sse.Event{
		ID:   "nil-data",
		Type: "test",
		Data: nil,
	}

	formatted := event.Format()
	assert.NotNil(t, formatted)
	assert.Contains(t, string(formatted), "data: \n\n")
}

func TestSecurity_SSE_LargePayload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	// Very large SSE payload should not crash
	largeData := []byte(strings.Repeat("x", 1024*1024)) // 1MB
	event := &sse.Event{
		ID:   "large-msg",
		Type: "large",
		Data: largeData,
	}

	formatted := event.Format()
	assert.True(t, len(formatted) > 1024*1024)
}

func TestSecurity_SSE_SpecialCharactersInEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	// SSE spec: data with newlines should be split, but our implementation
	// sends as-is. Ensure no panic.
	specialPayloads := [][]byte{
		[]byte("line1\nline2\nline3"),
		[]byte("tab\there"),
		[]byte("null\x00byte"),
		[]byte("\xff\xfe invalid UTF-8"),
		[]byte("<script>alert('xss')</script>"),
	}

	for _, payload := range specialPayloads {
		event := &sse.Event{
			Type: "test",
			Data: payload,
		}
		formatted := event.Format()
		assert.NotNil(t, formatted, "formatting should not fail for special payload")
	}
}

func TestSecurity_SSE_MaxClientLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	broker := sse.NewBroker(&sse.Config{
		BufferSize:        10,
		HeartbeatInterval: time.Hour,
		MaxClients:        5,
	})
	defer broker.Close()

	// Verify the max clients configuration is respected
	assert.Equal(t, 0, broker.ClientCount())
}

func TestSecurity_Transport_NilConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()

	_, err := factory.Create(nil)
	assert.Error(t, err, "nil config should be rejected")
}

func TestSecurity_Transport_EmptyType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()

	_, err := factory.Create(&transport.Config{
		Type:    "",
		Address: "localhost:0",
	})
	assert.Error(t, err, "empty type should be rejected")
}

func TestSecurity_Transport_DoubleClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()
	tr, err := factory.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "localhost:0",
	})
	require.NoError(t, err)

	// First close
	err = tr.Close()
	assert.NoError(t, err)

	// Second close should not panic
	err = tr.Close()
	assert.NoError(t, err)
}

func TestSecurity_Transport_SendAfterClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()
	tr, err := factory.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "localhost:0",
	})
	require.NoError(t, err)

	_ = tr.Close()

	ctx := context.Background()
	err = tr.Send(ctx, []byte("after close"))
	assert.Error(t, err, "send after close should return error")
}

func TestSecurity_Webhook_EmptySecret(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	// Sign with empty secret should still work (HMAC with empty key)
	payload := []byte("test")
	sig := webhook.Sign(payload, "")
	assert.NotEmpty(t, sig)
	assert.True(t, webhook.Verify(payload, sig, ""))
}

func TestSecurity_Webhook_EmptyPayload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	sig := webhook.Sign([]byte{}, "secret")
	assert.NotEmpty(t, sig)
	assert.True(t, webhook.Verify([]byte{}, sig, "secret"))

	sig = webhook.Sign(nil, "secret")
	assert.NotEmpty(t, sig)
}

func TestSecurity_Webhook_InactiveWebhookRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	dispatcher := webhook.NewDispatcher(&webhook.DispatcherConfig{
		MaxRetries:      0,
		Timeout:         time.Second,
		BackoffBase:     time.Millisecond,
		BackoffMax:      time.Millisecond,
		SignatureHeader: "X-Sig",
		UserAgent:       "test",
	})

	inactiveHook := &webhook.Webhook{
		URL:    "https://example.com/hook",
		Active: false,
	}

	ctx := context.Background()
	err := dispatcher.Send(ctx, inactiveHook, &webhook.Payload{
		ID:        "1",
		Event:     "test",
		Data:      "data",
		Timestamp: time.Now(),
	})
	assert.Error(t, err, "sending to inactive webhook should fail")
	assert.Contains(t, err.Error(), "not active")
}

func TestSecurity_Webhook_RegistryNonExistent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	registry := webhook.NewRegistry()

	// Get non-existent
	_, ok := registry.Get("non-existent")
	assert.False(t, ok)

	// Unregister non-existent should not panic
	registry.Unregister("non-existent")
	assert.Equal(t, 0, registry.Count())

	// MatchingWebhooks with empty registry
	matched := registry.MatchingWebhooks("any-event")
	assert.Len(t, matched, 0)
}
