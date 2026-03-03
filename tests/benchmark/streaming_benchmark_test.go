package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"digital.vasic.streaming/pkg/sse"
	"digital.vasic.streaming/pkg/transport"
	"digital.vasic.streaming/pkg/webhook"
)

func BenchmarkSSE_EventFormat(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	event := &sse.Event{
		ID:   "bench-id",
		Type: "benchmark",
		Data: []byte(`{"message":"benchmark payload data","seq":12345}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.Format()
	}
}

func BenchmarkSSE_EventFormat_LargePayload(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte('A' + (i % 26))
	}

	event := &sse.Event{
		ID:   "large-bench",
		Type: "large",
		Data: data,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.Format()
	}
}

func BenchmarkSSE_BrokerBroadcast(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	broker := sse.NewBroker(&sse.Config{
		BufferSize:        1000,
		HeartbeatInterval: time.Hour,
		MaxClients:        0,
	})
	defer broker.Close()

	event := &sse.Event{
		Type: "bench",
		Data: []byte("benchmark broadcast data"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.Broadcast(event)
	}
}

func BenchmarkTransport_CreateAndClose(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	factory := transport.NewFactory()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr, err := factory.Create(&transport.Config{
			Type:    transport.TypeHTTP,
			Address: "localhost:0",
		})
		if err != nil {
			b.Fatal(err)
		}
		_ = tr.Close()
	}
}

func BenchmarkTransport_Send(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	factory := transport.NewFactory()
	tr, err := factory.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "localhost:0",
		Options: map[string]interface{}{"buffer_size": b.N + 1000},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = tr.Close() }()

	ctx := context.Background()
	data := []byte("benchmark-transport-message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tr.Send(ctx, data)
	}
}

func BenchmarkWebhook_Sign(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	payload := []byte(`{"event":"user.created","user_id":"12345","timestamp":"2024-01-01T00:00:00Z"}`)
	secret := "webhook-secret-key-for-benchmarking"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = webhook.Sign(payload, secret)
	}
}

func BenchmarkWebhook_Verify(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	payload := []byte(`{"event":"user.created","user_id":"12345","timestamp":"2024-01-01T00:00:00Z"}`)
	secret := "webhook-secret-key-for-benchmarking"
	signature := webhook.Sign(payload, secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = webhook.Verify(payload, signature, secret)
	}
}

func BenchmarkWebhook_RegistryMatch(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	registry := webhook.NewRegistry()
	for i := 0; i < 100; i++ {
		registry.Register(fmt.Sprintf("wh-%d", i), &webhook.Webhook{
			URL:    fmt.Sprintf("https://example.com/hook/%d", i),
			Events: []string{fmt.Sprintf("event.type.%d", i%10)},
			Active: true,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.MatchingWebhooks(fmt.Sprintf("event.type.%d", i%10))
	}
}
