package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.streaming/pkg/sse"
	"digital.vasic.streaming/pkg/transport"
	"digital.vasic.streaming/pkg/webhook"
)

func TestSSE_EventFormatting_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	event := &sse.Event{
		ID:   "msg-001",
		Type: "message",
		Data: []byte(`{"text":"hello world"}`),
	}

	formatted := event.Format()
	expected := "id: msg-001\nevent: message\ndata: {\"text\":\"hello world\"}\n\n"
	assert.Equal(t, expected, string(formatted))
}

func TestSSE_EventWithRetry_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	event := &sse.Event{
		ID:    "msg-002",
		Type:  "update",
		Data:  []byte("data payload"),
		Retry: 5000,
	}

	formatted := event.Format()
	assert.Contains(t, string(formatted), "id: msg-002")
	assert.Contains(t, string(formatted), "event: update")
	assert.Contains(t, string(formatted), "retry: 5000")
	assert.Contains(t, string(formatted), "data: data payload")
}

func TestSSE_BrokerLifecycle_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	broker := sse.NewBroker(&sse.Config{
		BufferSize:        10,
		HeartbeatInterval: time.Hour, // long interval to avoid interference
		MaxClients:        100,
	})

	assert.Equal(t, 0, broker.ClientCount())

	// Close should not panic
	broker.Close()
	assert.Equal(t, 0, broker.ClientCount())
}

func TestTransport_FactoryLifecycle_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()
	types := factory.SupportedTypes()
	assert.True(t, len(types) >= 3, "should support HTTP, WebSocket, gRPC")

	// Create HTTP transport
	httpTransport, err := factory.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "localhost:8080",
	})
	require.NoError(t, err)
	require.NotNil(t, httpTransport)

	// Send and verify
	ctx := context.Background()
	err = httpTransport.Send(ctx, []byte("hello"))
	assert.NoError(t, err)

	// Close
	err = httpTransport.Close()
	assert.NoError(t, err)

	// Send after close should fail
	err = httpTransport.Send(ctx, []byte("after close"))
	assert.Error(t, err)
}

func TestTransport_WebSocketType_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()

	wsTransport, err := factory.Create(&transport.Config{
		Type:    transport.TypeWebSocket,
		Address: "ws://localhost:8081",
	})
	require.NoError(t, err)
	require.NotNil(t, wsTransport)

	ctx := context.Background()
	err = wsTransport.Send(ctx, []byte("ws-message"))
	assert.NoError(t, err)

	_ = wsTransport.Close()
}

func TestTransport_GRPCType_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()

	grpcTransport, err := factory.Create(&transport.Config{
		Type:    transport.TypeGRPC,
		Address: "localhost:50051",
	})
	require.NoError(t, err)
	require.NotNil(t, grpcTransport)

	ctx := context.Background()
	err = grpcTransport.Send(ctx, []byte("grpc-message"))
	assert.NoError(t, err)

	_ = grpcTransport.Close()
}

func TestTransport_UnsupportedType_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	factory := transport.NewFactory()

	_, err := factory.Create(&transport.Config{
		Type:    "mqtt",
		Address: "localhost:1883",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported transport type")
}

func TestWebhook_SignVerify_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	secret := "my-webhook-secret-key"
	payload := []byte(`{"event":"user.created","id":"123"}`)

	signature := webhook.Sign(payload, secret)
	assert.True(t, len(signature) > 0)
	assert.Contains(t, signature, "sha256=")

	// Verify with correct secret
	assert.True(t, webhook.Verify(payload, signature, secret))

	// Verify with wrong secret should fail
	assert.False(t, webhook.Verify(payload, signature, "wrong-secret"))

	// Verify with tampered payload should fail
	tampered := []byte(`{"event":"user.created","id":"999"}`)
	assert.False(t, webhook.Verify(tampered, signature, secret))
}

func TestWebhook_RegistryLifecycle_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	registry := webhook.NewRegistry()
	assert.Equal(t, 0, registry.Count())

	// Register webhooks
	registry.Register("wh-1", &webhook.Webhook{
		URL:    "https://example.com/hook1",
		Secret: "secret1",
		Events: []string{"user.created"},
		Active: true,
	})
	registry.Register("wh-2", &webhook.Webhook{
		URL:    "https://example.com/hook2",
		Events: []string{"user.deleted"},
		Active: true,
	})
	registry.Register("wh-3", &webhook.Webhook{
		URL:    "https://example.com/hook3",
		Events: []string{"*"},
		Active: false,
	})
	assert.Equal(t, 3, registry.Count())

	// Get
	wh, ok := registry.Get("wh-1")
	assert.True(t, ok)
	assert.Equal(t, "https://example.com/hook1", wh.URL)

	// List
	all := registry.List()
	assert.Len(t, all, 3)

	// MatchingWebhooks: user.created should match wh-1
	matched := registry.MatchingWebhooks("user.created")
	assert.Len(t, matched, 1)
	assert.Equal(t, "https://example.com/hook1", matched[0].URL)

	// MatchingWebhooks: user.deleted should match wh-2
	matched = registry.MatchingWebhooks("user.deleted")
	assert.Len(t, matched, 1)

	// wh-3 is inactive, should not match even with wildcard
	matched = registry.MatchingWebhooks("anything")
	assert.Len(t, matched, 0)

	// Unregister
	registry.Unregister("wh-1")
	assert.Equal(t, 2, registry.Count())

	_, ok = registry.Get("wh-1")
	assert.False(t, ok)
}
