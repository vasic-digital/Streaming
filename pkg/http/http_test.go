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
