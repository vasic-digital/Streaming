package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSign(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		secret  string
	}{
		{
			name:    "simple payload",
			payload: []byte(`{"event":"test"}`),
			secret:  "mysecret",
		},
		{
			name:    "empty payload",
			payload: []byte(`{}`),
			secret:  "secret123",
		},
		{
			name:    "complex payload",
			payload: []byte(`{"id":"123","data":{"key":"value","num":42}}`),
			secret:  "complex-secret-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := Sign(tt.payload, tt.secret)
			assert.NotEmpty(t, sig)
			assert.Contains(t, sig, "sha256=")
			// Signature should be deterministic
			sig2 := Sign(tt.payload, tt.secret)
			assert.Equal(t, sig, sig2)
		})
	}
}

func TestVerify(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		secret    string
		signature string
		expected  bool
	}{
		{
			name:      "valid signature",
			payload:   []byte(`{"event":"test"}`),
			secret:    "mysecret",
			signature: Sign([]byte(`{"event":"test"}`), "mysecret"),
			expected:  true,
		},
		{
			name:      "invalid signature",
			payload:   []byte(`{"event":"test"}`),
			secret:    "mysecret",
			signature: "sha256=invalid",
			expected:  false,
		},
		{
			name:      "wrong secret",
			payload:   []byte(`{"event":"test"}`),
			secret:    "wrongsecret",
			signature: Sign([]byte(`{"event":"test"}`), "mysecret"),
			expected:  false,
		},
		{
			name:      "tampered payload",
			payload:   []byte(`{"event":"tampered"}`),
			secret:    "mysecret",
			signature: Sign([]byte(`{"event":"test"}`), "mysecret"),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Verify(tt.payload, tt.signature, tt.secret)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultDispatcherConfig(t *testing.T) {
	config := DefaultDispatcherConfig()
	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, time.Second, config.BackoffBase)
	assert.Equal(t, 5*time.Minute, config.BackoffMax)
	assert.Equal(t, "X-Signature-256", config.SignatureHeader)
	assert.Equal(t, "Streaming-Webhook/1.0", config.UserAgent)
}

func TestNewDispatcher(t *testing.T) {
	tests := []struct {
		name   string
		config *DispatcherConfig
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name: "custom config",
			config: &DispatcherConfig{
				MaxRetries:  3,
				Timeout:     10 * time.Second,
				BackoffBase: 500 * time.Millisecond,
				BackoffMax:  30 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDispatcher(tt.config)
			require.NotNil(t, d)
		})
	}
}

func TestDispatcher_Send_Success(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := json.Marshal(map[string]string{"status": "ok"})
			receivedBody, _ = json.Marshal(r.Body)
			_ = receivedBody
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		},
	))
	defer server.Close()

	d := NewDispatcher(&DispatcherConfig{
		MaxRetries:      1,
		Timeout:         5 * time.Second,
		BackoffBase:     100 * time.Millisecond,
		BackoffMax:      time.Second,
		SignatureHeader: "X-Sig",
		UserAgent:       "test",
	})

	webhook := &Webhook{
		URL:    server.URL,
		Secret: "test-secret",
		Active: true,
	}

	payload := &Payload{
		ID:        "delivery-1",
		Event:     "test.event",
		Data:      map[string]string{"key": "value"},
		Timestamp: time.Now(),
	}

	err := d.Send(context.Background(), webhook, payload)
	require.NoError(t, err)

	delivered, failed := d.Stats()
	assert.Equal(t, int64(1), delivered)
	assert.Equal(t, int64(0), failed)
}

func TestDispatcher_Send_Inactive(t *testing.T) {
	d := NewDispatcher(nil)

	webhook := &Webhook{
		URL:    "http://example.com",
		Active: false,
	}

	payload := &Payload{
		ID:    "d1",
		Event: "test",
	}

	err := d.Send(context.Background(), webhook, payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

func TestDispatcher_Send_ServerError_WithRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	defer server.Close()

	d := NewDispatcher(&DispatcherConfig{
		MaxRetries:  2,
		Timeout:     5 * time.Second,
		BackoffBase: 10 * time.Millisecond,
		BackoffMax:  50 * time.Millisecond,
	})

	webhook := &Webhook{URL: server.URL, Active: true}
	payload := &Payload{ID: "retry-test", Event: "test"}

	err := d.Send(context.Background(), webhook, payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after")

	// 1 initial + 2 retries = 3 total attempts
	assert.Equal(t, int32(3), attempts.Load())

	_, failed := d.Stats()
	assert.Equal(t, int64(1), failed)
}

func TestDispatcher_Send_EventualSuccess(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	d := NewDispatcher(&DispatcherConfig{
		MaxRetries:  5,
		Timeout:     5 * time.Second,
		BackoffBase: 10 * time.Millisecond,
		BackoffMax:  50 * time.Millisecond,
	})

	webhook := &Webhook{URL: server.URL, Active: true}
	payload := &Payload{ID: "eventual", Event: "test"}

	err := d.Send(context.Background(), webhook, payload)
	require.NoError(t, err)

	assert.Equal(t, int32(3), attempts.Load())

	delivered, _ := d.Stats()
	assert.Equal(t, int64(1), delivered)
}

func TestDispatcher_Send_WithSignature(t *testing.T) {
	var receivedSig string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			receivedSig = r.Header.Get("X-Signature-256")
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	d := NewDispatcher(nil)

	webhook := &Webhook{
		URL:    server.URL,
		Secret: "sign-secret",
		Active: true,
	}

	payload := &Payload{
		ID:    "signed",
		Event: "test",
		Data:  "data",
	}

	err := d.Send(context.Background(), webhook, payload)
	require.NoError(t, err)
	assert.NotEmpty(t, receivedSig)
	assert.Contains(t, receivedSig, "sha256=")
}

func TestDispatcher_Send_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	defer server.Close()

	d := NewDispatcher(&DispatcherConfig{
		MaxRetries:  10,
		Timeout:     5 * time.Second,
		BackoffBase: 1 * time.Second,
		BackoffMax:  5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())

	webhook := &Webhook{URL: server.URL, Active: true}
	payload := &Payload{ID: "cancel", Event: "test"}

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := d.Send(ctx, webhook, payload)
	assert.Error(t, err)
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	assert.Equal(t, 0, r.Count())
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()

	tests := []struct {
		name    string
		id      string
		webhook *Webhook
	}{
		{
			name: "basic webhook",
			id:   "wh1",
			webhook: &Webhook{
				URL:    "http://example.com/hook",
				Active: true,
			},
		},
		{
			name: "webhook with secret",
			id:   "wh2",
			webhook: &Webhook{
				URL:    "http://example.com/secure",
				Secret: "secret123",
				Events: []string{"push", "pull"},
				Active: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.Register(tt.id, tt.webhook)
			wh, ok := r.Get(tt.id)
			assert.True(t, ok)
			assert.Equal(t, tt.webhook.URL, wh.URL)
		})
	}

	assert.Equal(t, 2, r.Count())
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	r.Register("wh1", &Webhook{URL: "http://a.com", Active: true})
	r.Register("wh2", &Webhook{URL: "http://b.com", Active: true})

	assert.Equal(t, 2, r.Count())

	r.Unregister("wh1")
	assert.Equal(t, 1, r.Count())

	_, ok := r.Get("wh1")
	assert.False(t, ok)

	// Unregister non-existent is no-op
	r.Unregister("nonexistent")
	assert.Equal(t, 1, r.Count())
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register("a", &Webhook{URL: "http://a.com", Active: true})
	r.Register("b", &Webhook{URL: "http://b.com", Active: false})

	list := r.List()
	assert.Len(t, list, 2)
}

func TestRegistry_MatchingWebhooks(t *testing.T) {
	r := NewRegistry()
	r.Register("all", &Webhook{URL: "http://all.com", Active: true})
	r.Register("push-only", &Webhook{
		URL:    "http://push.com",
		Events: []string{"push"},
		Active: true,
	})
	r.Register("wildcard", &Webhook{
		URL:    "http://wild.com",
		Events: []string{"*"},
		Active: true,
	})
	r.Register("inactive", &Webhook{
		URL:    "http://inactive.com",
		Active: false,
	})

	tests := []struct {
		name     string
		event    string
		expected int
	}{
		{
			name:     "push matches all, push-only, wildcard",
			event:    "push",
			expected: 3,
		},
		{
			name:     "pull matches all, wildcard",
			event:    "pull",
			expected: 2,
		},
		{
			name:     "unknown matches all, wildcard",
			event:    "unknown",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := r.MatchingWebhooks(tt.event)
			assert.Len(t, matched, tt.expected)
		})
	}
}

func TestPayload_JSON(t *testing.T) {
	tests := []struct {
		name    string
		payload Payload
	}{
		{
			name: "basic payload",
			payload: Payload{
				ID:        "p1",
				Event:     "test",
				Timestamp: time.Now(),
			},
		},
		{
			name: "payload with data",
			payload: Payload{
				ID:    "p2",
				Event: "update",
				Data:  map[string]interface{}{"key": "value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			var decoded Payload
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.payload.ID, decoded.ID)
			assert.Equal(t, tt.payload.Event, decoded.Event)
		})
	}
}

func TestDispatcher_Send_InvalidURL(t *testing.T) {
	// Test the path where http.NewRequestWithContext fails due to invalid URL
	d := NewDispatcher(&DispatcherConfig{
		MaxRetries:  0,
		Timeout:     5 * time.Second,
		BackoffBase: 10 * time.Millisecond,
		BackoffMax:  50 * time.Millisecond,
	})

	webhook := &Webhook{
		URL:    "://invalid-url", // Invalid URL scheme
		Active: true,
	}

	payload := &Payload{ID: "invalid", Event: "test"}

	err := d.Send(context.Background(), webhook, payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestDispatcher_calculateBackoff_ExceedsMax(t *testing.T) {
	// Test the backoff capping logic
	d := NewDispatcher(&DispatcherConfig{
		MaxRetries:  10,
		Timeout:     5 * time.Second,
		BackoffBase: time.Second,
		BackoffMax:  5 * time.Second,
	})

	// At attempt 5, backoff would be 1s * 2^4 = 16s, should be capped at 5s
	backoff := d.calculateBackoff(5)
	assert.Equal(t, 5*time.Second, backoff)

	// At attempt 10, backoff would be 1s * 2^9 = 512s, should be capped at 5s
	backoff = d.calculateBackoff(10)
	assert.Equal(t, 5*time.Second, backoff)
}

func TestDispatcher_Send_NoSecret(t *testing.T) {
	// Test Send without a secret (no signature header set)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			sig := r.Header.Get("X-Signature-256")
			// No signature should be present
			if sig != "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	d := NewDispatcher(nil)

	webhook := &Webhook{
		URL:    server.URL,
		Secret: "", // No secret
		Active: true,
	}

	payload := &Payload{ID: "no-secret", Event: "test"}

	err := d.Send(context.Background(), webhook, payload)
	require.NoError(t, err)
}

func TestDispatcher_Send_MarshalError(t *testing.T) {
	// Test the path where json.Marshal fails
	d := NewDispatcher(nil)

	webhook := &Webhook{
		URL:    "http://example.com",
		Active: true,
	}

	// Create a payload with unmarshalable data (channel)
	payload := &Payload{
		ID:    "marshal-error",
		Event: "test",
		Data:  make(chan int), // Channels can't be marshaled
	}

	err := d.Send(context.Background(), webhook, payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal payload")
}

func TestDispatcher_doSend_RequestError(t *testing.T) {
	// Test when the HTTP client returns an error (connection refused)
	d := NewDispatcher(&DispatcherConfig{
		MaxRetries:  0,
		Timeout:     100 * time.Millisecond,
		BackoffBase: 10 * time.Millisecond,
		BackoffMax:  50 * time.Millisecond,
	})

	webhook := &Webhook{
		URL:    "http://localhost:59999", // Port that's not listening
		Active: true,
	}

	payload := &Payload{ID: "req-error", Event: "test"}

	err := d.Send(context.Background(), webhook, payload)
	assert.Error(t, err)
	// The error will be from the HTTP client failing to connect
}
