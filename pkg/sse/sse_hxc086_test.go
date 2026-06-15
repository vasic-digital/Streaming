package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBroker_AddClient_FallbackID_Unique_HXC086 reproduces HXC-086: when the
// X-Client-ID header is absent, the broker generated the fallback client ID
// solely from time.Now().UnixNano() (sse.go:140). Two concurrent SSE connects
// landing on the same nanosecond tick produced an identical map key, so the
// second client silently overwrote the first in b.clients (sse.go:153) —
// losing a client (plus leaking its goroutine/channel).
//
// This drives the REAL AddClient fallback path concurrently for N clients with
// NO X-Client-ID header and asserts every registered client is still present
// in the broker. On the pre-fix code this FAILS (b.clients has fewer than N
// entries because of UnixNano collisions). Run WITHOUT -race so the scheduler
// produces realistic same-tick concurrency (the proven repro style).
func TestBroker_AddClient_FallbackID_Unique_HXC086(t *testing.T) {
	const n = 1000

	cfg := DefaultConfig()
	cfg.MaxClients = 0 // unlimited — isolate the ID-collision defect
	broker := NewBroker(cfg)
	defer broker.Close()

	// Each connection gets a cancellable context so AddClient's disconnect
	// goroutine stays parked (no RemoveClient) for the duration of the assert.
	cancels := make([]context.CancelFunc, 0, n)
	var cancelsMu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			cancelsMu.Lock()
			cancels = append(cancels, cancel)
			cancelsMu.Unlock()

			w := newMockFlusherResponseWriter()
			r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
			// NO X-Client-ID header -> fallback ID generation path.

			<-start // maximize same-nanosecond-tick contention
			_, err := broker.AddClient(w, r)
			require.NoError(t, err)
		}()
	}
	close(start)
	wg.Wait()

	// Tear down all connections after the assertion.
	defer func() {
		cancelsMu.Lock()
		defer cancelsMu.Unlock()
		for _, c := range cancels {
			c()
		}
	}()

	broker.clientsMu.RLock()
	got := len(broker.clients)
	broker.clientsMu.RUnlock()

	require.Equalf(t, n, got,
		"HXC-086: expected all %d fallback-ID clients present in broker, "+
			"got %d — %d client(s) silently lost to UnixNano ID collision",
		n, got, n-got)
}
