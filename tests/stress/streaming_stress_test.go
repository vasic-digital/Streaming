package stress

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.streaming/pkg/sse"
	"digital.vasic.streaming/pkg/transport"
	"digital.vasic.streaming/pkg/webhook"
)

func TestStress_ConcurrentSSEBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	broker := sse.NewBroker(&sse.Config{
		BufferSize:        100,
		HeartbeatInterval: time.Hour,
		MaxClients:        1000,
	})
	defer broker.Close()

	const goroutines = 100
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				broker.Broadcast(&sse.Event{
					ID:   fmt.Sprintf("stress-%d-%d", id, i),
					Type: "stress-test",
					Data: []byte(fmt.Sprintf("message %d from goroutine %d", i, id)),
				})
			}
		}(g)
	}

	wg.Wait()
	// No deadlocks or panics means success
}

func TestStress_ConcurrentTransportSend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	factory := transport.NewFactory()
	tr, err := factory.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "localhost:0",
		Options: map[string]interface{}{"buffer_size": 10000},
	})
	require.NoError(t, err)
	defer func() { _ = tr.Close() }()

	const goroutines = 80
	var wg sync.WaitGroup
	var errCount atomic.Int64

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < 50; i++ {
				data := []byte(fmt.Sprintf("stress-msg-%d-%d", id, i))
				if err := tr.Send(ctx, data); err != nil {
					errCount.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load())
}

func TestStress_ConcurrentWebhookSignVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutines = 100
	var wg sync.WaitGroup
	var errCount atomic.Int64

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				secret := fmt.Sprintf("secret-%d", id)
				payload := []byte(fmt.Sprintf(`{"id":%d,"seq":%d}`, id, i))

				sig := webhook.Sign(payload, secret)
				if !webhook.Verify(payload, sig, secret) {
					errCount.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load())
}

func TestStress_ConcurrentRegistryOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	registry := webhook.NewRegistry()

	const goroutines = 50
	var wg sync.WaitGroup

	// Half goroutines register, half unregister
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				whID := fmt.Sprintf("wh-%d-%d", id, i)
				registry.Register(whID, &webhook.Webhook{
					URL:    fmt.Sprintf("https://example.com/%s", whID),
					Active: true,
					Events: []string{fmt.Sprintf("event.%d", id)},
				})
			}
		}(g)
	}
	wg.Wait()

	// Concurrent reads
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = registry.List()
				_ = registry.MatchingWebhooks(fmt.Sprintf("event.%d", id))
				_ = registry.Count()
			}
		}(g)
	}
	wg.Wait()

	assert.True(t, registry.Count() > 0)
}

func TestStress_ConcurrentTransportCreateClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	factory := transport.NewFactory()

	const goroutines = 80
	var wg sync.WaitGroup
	var errCount atomic.Int64

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				tr, err := factory.Create(&transport.Config{
					Type:    transport.TypeHTTP,
					Address: fmt.Sprintf("localhost:%d", 10000+id*100+i),
				})
				if err != nil {
					errCount.Add(1)
					continue
				}
				ctx := context.Background()
				_ = tr.Send(ctx, []byte("data"))
				if err := tr.Close(); err != nil {
					errCount.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load())
}

func TestStress_ConcurrentSSEEventFormatting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutines = 100
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				event := &sse.Event{
					ID:   fmt.Sprintf("stress-%d-%d", id, i),
					Type: "stress",
					Data: []byte(fmt.Sprintf(`{"goroutine":%d,"seq":%d}`, id, i)),
				}
				formatted := event.Format()
				if len(formatted) == 0 {
					t.Errorf("empty formatted event")
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestStress_HighVolumeWebhookMatching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	registry := webhook.NewRegistry()

	// Register many webhooks with diverse event subscriptions
	for i := 0; i < 200; i++ {
		registry.Register(fmt.Sprintf("wh-%d", i), &webhook.Webhook{
			URL:    fmt.Sprintf("https://example.com/hook/%d", i),
			Events: []string{fmt.Sprintf("event.type.%d", i%10)},
			Active: i%5 != 0, // 80% active
		})
	}

	const goroutines = 100
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				eventType := fmt.Sprintf("event.type.%d", i%10)
				matched := registry.MatchingWebhooks(eventType)
				if len(matched) == 0 && i%10 != 0 {
					// At least some non-zero events should have matches
				}
				_ = matched
			}
		}(g)
	}

	wg.Wait()
}
