package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.streaming/pkg/sse"
	"digital.vasic.streaming/pkg/transport"
	"digital.vasic.streaming/pkg/webhook"
)

func TestEndToEnd_SSEBrokerBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	broker := sse.NewBroker(&sse.Config{
		BufferSize:        50,
		HeartbeatInterval: time.Hour,
		MaxClients:        10,
	})
	defer broker.Close()

	// Broadcast to zero clients should not panic
	broker.Broadcast(&sse.Event{
		Type: "test",
		Data: []byte("broadcast to nobody"),
	})

	assert.Equal(t, 0, broker.ClientCount())
}

func TestEndToEnd_SSEEventTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testCases := []struct {
		name     string
		event    *sse.Event
		expected []string
	}{
		{
			name:     "data only",
			event:    &sse.Event{Data: []byte("hello")},
			expected: []string{"data: hello\n\n"},
		},
		{
			name:     "with type",
			event:    &sse.Event{Type: "message", Data: []byte("typed")},
			expected: []string{"event: message\n", "data: typed\n\n"},
		},
		{
			name:     "with id and retry",
			event:    &sse.Event{ID: "42", Type: "update", Data: []byte("data"), Retry: 3000},
			expected: []string{"id: 42\n", "event: update\n", "retry: 3000\n", "data: data\n\n"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			formatted := string(tc.event.Format())
			for _, exp := range tc.expected {
				assert.Contains(t, formatted, exp)
			}
		})
	}
}

func TestEndToEnd_TransportFactory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	factory := transport.NewFactory()

	// Verify all types can be created
	types := []transport.Type{
		transport.TypeHTTP,
		transport.TypeWebSocket,
		transport.TypeGRPC,
	}

	for _, tp := range types {
		t.Run(string(tp), func(t *testing.T) {
			tr, err := factory.Create(&transport.Config{
				Type:    tp,
				Address: "localhost:0",
			})
			require.NoError(t, err)
			require.NotNil(t, tr)

			ctx := context.Background()
			err = tr.Send(ctx, []byte("test-"+string(tp)))
			assert.NoError(t, err)

			err = tr.Close()
			assert.NoError(t, err)
		})
	}
}

func TestEndToEnd_TransportCustomRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	factory := transport.NewFactory()

	// Register custom transport type
	customType := transport.Type("custom-protocol")
	factory.Register(customType, func(cfg *transport.Config) (transport.Transport, error) {
		return &mockTransport{address: cfg.Address}, nil
	})

	types := factory.SupportedTypes()
	found := false
	for _, tp := range types {
		if tp == customType {
			found = true
			break
		}
	}
	assert.True(t, found, "custom transport type should be registered")

	tr, err := factory.Create(&transport.Config{
		Type:    customType,
		Address: "custom://localhost",
	})
	require.NoError(t, err)
	require.NotNil(t, tr)

	_ = tr.Close()
}

func TestEndToEnd_WebhookRegistryMatching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	registry := webhook.NewRegistry()

	// Register webhooks with various event subscriptions
	for i := 0; i < 10; i++ {
		registry.Register(fmt.Sprintf("wh-%d", i), &webhook.Webhook{
			URL:    fmt.Sprintf("https://example.com/hook/%d", i),
			Events: []string{fmt.Sprintf("event.type.%d", i%3)},
			Active: true,
		})
	}

	// Add wildcard webhook
	registry.Register("wh-wildcard", &webhook.Webhook{
		URL:    "https://example.com/wildcard",
		Events: []string{"*"},
		Active: true,
	})

	// Add catch-all (empty events)
	registry.Register("wh-catchall", &webhook.Webhook{
		URL:    "https://example.com/catchall",
		Active: true,
	})

	assert.Equal(t, 12, registry.Count())

	// Match event.type.0 should get specific + wildcard + catchall
	matched := registry.MatchingWebhooks("event.type.0")
	assert.True(t, len(matched) >= 3, "should match at least 3 webhooks, got %d", len(matched))

	// Unknown event should match wildcard + catchall
	matched = registry.MatchingWebhooks("unknown.event")
	assert.Equal(t, 2, len(matched))
}

func TestEndToEnd_WebhookDispatcherStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	dispatcher := webhook.NewDispatcher(&webhook.DispatcherConfig{
		MaxRetries:      0,
		Timeout:         time.Second,
		BackoffBase:     100 * time.Millisecond,
		BackoffMax:      time.Second,
		SignatureHeader: "X-Test-Signature",
		UserAgent:       "Test/1.0",
	})

	delivered, failed := dispatcher.Stats()
	assert.Equal(t, int64(0), delivered)
	assert.Equal(t, int64(0), failed)
}

func TestEndToEnd_TransportContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	factory := transport.NewFactory()
	tr, err := factory.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "localhost:0",
		Options: map[string]interface{}{"buffer_size": 1},
	})
	require.NoError(t, err)

	// Cancel context before receive
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tr.Receive(ctx)
	assert.Error(t, err, "receive with cancelled context should fail")

	_ = tr.Close()
}

func TestEndToEnd_WebhookSignatureChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	// Verify sign/verify consistency across multiple payloads
	secrets := []string{"secret-1", "secret-2", "another-secret"}
	payloads := [][]byte{
		[]byte(`{"id":1}`),
		[]byte(`{"id":2,"nested":{"key":"value"}}`),
		[]byte("plain text payload"),
		[]byte(""),
	}

	for _, secret := range secrets {
		for _, payload := range payloads {
			sig := webhook.Sign(payload, secret)
			assert.True(t, webhook.Verify(payload, sig, secret),
				"verify should succeed for secret=%q payload=%q", secret, string(payload))

			// Cross-secret should fail
			for _, otherSecret := range secrets {
				if otherSecret != secret {
					assert.False(t, webhook.Verify(payload, sig, otherSecret),
						"cross-secret verify should fail")
				}
			}
		}
	}
}

// mockTransport is a simple transport for custom registration testing.
type mockTransport struct {
	address string
	closed  bool
}

func (m *mockTransport) Send(_ context.Context, _ []byte) error {
	if m.closed {
		return fmt.Errorf("transport closed")
	}
	return nil
}

func (m *mockTransport) Receive(_ context.Context) ([]byte, error) {
	if m.closed {
		return nil, fmt.Errorf("transport closed")
	}
	return nil, fmt.Errorf("no data")
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}
