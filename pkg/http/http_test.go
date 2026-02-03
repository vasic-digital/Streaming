package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, time.Second, config.BackoffBase)
	assert.Equal(t, 30*time.Second, config.BackoffMax)
	assert.Equal(t, 5, config.CircuitBreakerThreshold)
	assert.Equal(t, 60*time.Second, config.CircuitBreakerTimeout)
}

func TestNewClient(t *testing.T) {
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
				MaxRetries:              5,
				Timeout:                 10 * time.Second,
				BackoffBase:             500 * time.Millisecond,
				BackoffMax:              10 * time.Second,
				CircuitBreakerThreshold: 3,
				CircuitBreakerTimeout:   30 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.config)
			require.NotNil(t, c)
			assert.Equal(t, CircuitClosed, c.CircuitState())
		})
	}
}

func TestClient_Request_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		},
	))
	defer server.Close()

	c := NewClient(&Config{
		MaxRetries:              3,
		Timeout:                 5 * time.Second,
		BackoffBase:             10 * time.Millisecond,
		BackoffMax:              100 * time.Millisecond,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	resp, err := c.Request(context.Background(), "GET", server.URL, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.IsSuccess())
	assert.Equal(t, CircuitClosed, c.CircuitState())

	body, err := resp.BodyBytes()
	require.NoError(t, err)
	assert.Contains(t, string(body), "ok")
}

func TestClient_Request_RetryOnServerError(t *testing.T) {
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

	c := NewClient(&Config{
		MaxRetries:              5,
		Timeout:                 5 * time.Second,
		BackoffBase:             10 * time.Millisecond,
		BackoffMax:              50 * time.Millisecond,
		CircuitBreakerThreshold: 10,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	resp, err := c.Request(context.Background(), "GET", server.URL, nil)
	require.NoError(t, err)
	assert.True(t, resp.IsSuccess())
	assert.Equal(t, int32(3), attempts.Load())
}

func TestClient_Request_MaxRetriesExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	))
	defer server.Close()

	c := NewClient(&Config{
		MaxRetries:              2,
		Timeout:                 5 * time.Second,
		BackoffBase:             10 * time.Millisecond,
		BackoffMax:              50 * time.Millisecond,
		CircuitBreakerThreshold: 10,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	resp, err := c.Request(context.Background(), "GET", server.URL, nil)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "failed after")
}

func TestClient_Request_NonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad request"}`))
		},
	))
	defer server.Close()

	c := NewClient(&Config{
		MaxRetries:              3,
		Timeout:                 5 * time.Second,
		BackoffBase:             10 * time.Millisecond,
		BackoffMax:              50 * time.Millisecond,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	resp, err := c.Request(context.Background(), "GET", server.URL, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.False(t, resp.IsSuccess())
	assert.False(t, resp.IsRetryable())
}

func TestClient_Request_WithBody(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := (&Response{Response: &http.Response{Body: r.Body}}).BodyBytes()
			receivedBody = string(body)
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	c := NewClient(&Config{
		MaxRetries:              1,
		Timeout:                 5 * time.Second,
		BackoffBase:             10 * time.Millisecond,
		BackoffMax:              50 * time.Millisecond,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	body := strings.NewReader(`{"key":"value"}`)
	resp, err := c.Request(
		context.Background(), "POST", server.URL, body,
	)
	require.NoError(t, err)
	assert.True(t, resp.IsSuccess())
	assert.Equal(t, `{"key":"value"}`, receivedBody)
}

func TestClient_Request_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	))
	defer server.Close()

	c := NewClient(&Config{
		MaxRetries:              10,
		Timeout:                 5 * time.Second,
		BackoffBase:             time.Second,
		BackoffMax:              5 * time.Second,
		CircuitBreakerThreshold: 20,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	resp, err := c.Request(ctx, "GET", server.URL, nil)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := newCircuitBreaker(3, time.Minute)

	assert.Equal(t, CircuitClosed, cb.State())
	assert.True(t, cb.allow())

	cb.recordFailure()
	cb.recordFailure()
	assert.Equal(t, CircuitClosed, cb.State())

	cb.recordFailure()
	assert.Equal(t, CircuitOpen, cb.State())
	assert.False(t, cb.allow())
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := newCircuitBreaker(2, 50*time.Millisecond)

	cb.recordFailure()
	cb.recordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.allow())
	assert.Equal(t, CircuitHalfOpen, cb.State())
}

func TestCircuitBreaker_ClosesOnSuccess(t *testing.T) {
	cb := newCircuitBreaker(2, 50*time.Millisecond)

	cb.recordFailure()
	cb.recordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	time.Sleep(60 * time.Millisecond)
	_ = cb.allow() // transitions to half-open

	cb.recordSuccess()
	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 0, cb.failures)
}

func TestResponse_BodyBytes_NilBody(t *testing.T) {
	resp := &Response{
		Response: &http.Response{
			Body: nil,
		},
	}
	body, err := resp.BodyBytes()
	require.NoError(t, err)
	assert.Nil(t, body)
}

func TestResponse_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{"200 OK", 200, false},
		{"400 Bad Request", 400, false},
		{"401 Unauthorized", 401, false},
		{"404 Not Found", 404, false},
		{"429 Too Many Requests", 429, true},
		{"500 Internal Server Error", 500, false},
		{"502 Bad Gateway", 502, true},
		{"503 Service Unavailable", 503, true},
		{"504 Gateway Timeout", 504, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{
				Response: &http.Response{StatusCode: tt.statusCode},
			}
			assert.Equal(t, tt.expected, resp.IsRetryable())
		})
	}
}

func TestClient_Request_InvalidURL(t *testing.T) {
	c := NewClient(&Config{
		MaxRetries:              1,
		Timeout:                 5 * time.Second,
		BackoffBase:             10 * time.Millisecond,
		BackoffMax:              50 * time.Millisecond,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	// Invalid URL with control character triggers NewRequestWithContext error
	resp, err := c.Request(context.Background(), "GET", "http://[::1]:namedport", nil)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestClient_Request_NetworkError(t *testing.T) {
	c := NewClient(&Config{
		MaxRetries:              1,
		Timeout:                 100 * time.Millisecond,
		BackoffBase:             10 * time.Millisecond,
		BackoffMax:              50 * time.Millisecond,
		CircuitBreakerThreshold: 10,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	// Connect to a port that is not listening to trigger network error
	resp, err := c.Request(context.Background(), "GET", "http://127.0.0.1:1", nil)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "request failed")
}

func TestClient_Request_CircuitBreakerBlocksRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	))
	defer server.Close()

	c := NewClient(&Config{
		MaxRetries:              10,
		Timeout:                 5 * time.Second,
		BackoffBase:             5 * time.Millisecond,
		BackoffMax:              20 * time.Millisecond,
		CircuitBreakerThreshold: 2, // Opens after 2 failures
		CircuitBreakerTimeout:   10 * time.Second,
	})

	resp, err := c.Request(context.Background(), "GET", server.URL, nil)
	assert.Error(t, err)
	assert.Nil(t, resp)
	// After 2 failures, circuit breaker opens and blocks remaining retries
	// The error should mention circuit breaker
	assert.Contains(t, err.Error(), "circuit breaker is open")
}

func TestCircuitBreaker_AllowInHalfOpenState(t *testing.T) {
	cb := newCircuitBreaker(2, 10*time.Millisecond)

	// Trip the circuit breaker
	cb.recordFailure()
	cb.recordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	// Wait for timeout to transition to half-open
	time.Sleep(15 * time.Millisecond)
	assert.True(t, cb.allow()) // Transitions to half-open
	assert.Equal(t, CircuitHalfOpen, cb.State())

	// Subsequent calls in half-open should still allow
	assert.True(t, cb.allow())
	assert.Equal(t, CircuitHalfOpen, cb.State())
}

func TestClient_calculateBackoff_CappedAtMax(t *testing.T) {
	c := NewClient(&Config{
		MaxRetries:              10,
		Timeout:                 5 * time.Second,
		BackoffBase:             100 * time.Millisecond,
		BackoffMax:              500 * time.Millisecond,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   10 * time.Second,
	})

	// Attempt 1: 100ms * 2^0 = 100ms
	assert.Equal(t, 100*time.Millisecond, c.calculateBackoff(1))

	// Attempt 2: 100ms * 2^1 = 200ms
	assert.Equal(t, 200*time.Millisecond, c.calculateBackoff(2))

	// Attempt 3: 100ms * 2^2 = 400ms
	assert.Equal(t, 400*time.Millisecond, c.calculateBackoff(3))

	// Attempt 4: 100ms * 2^3 = 800ms > 500ms max, capped to 500ms
	assert.Equal(t, 500*time.Millisecond, c.calculateBackoff(4))

	// Attempt 5: 100ms * 2^4 = 1600ms > 500ms max, capped to 500ms
	assert.Equal(t, 500*time.Millisecond, c.calculateBackoff(5))
}

func TestResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{"199 Informational", 199, false},
		{"200 OK", 200, true},
		{"201 Created", 201, true},
		{"204 No Content", 204, true},
		{"299 Custom 2xx", 299, true},
		{"300 Multiple Choices", 300, false},
		{"400 Bad Request", 400, false},
		{"500 Internal Server Error", 500, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{
				Response: &http.Response{StatusCode: tt.statusCode},
			}
			assert.Equal(t, tt.expected, resp.IsSuccess())
		})
	}
}
