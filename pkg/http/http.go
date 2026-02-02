// Package http provides an HTTP client with automatic retry, exponential
// backoff, configurable timeouts, and a circuit breaker.
package http

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

// Config holds configuration for the HTTP Client.
type Config struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// Timeout is the per-request timeout.
	Timeout time.Duration
	// BackoffBase is the base duration for exponential backoff.
	BackoffBase time.Duration
	// BackoffMax is the maximum backoff duration.
	BackoffMax time.Duration
	// CircuitBreakerThreshold is the number of consecutive failures
	// before the circuit breaker opens.
	CircuitBreakerThreshold int
	// CircuitBreakerTimeout is how long the circuit stays open.
	CircuitBreakerTimeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:              3,
		Timeout:                 30 * time.Second,
		BackoffBase:             time.Second,
		BackoffMax:              30 * time.Second,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   60 * time.Second,
	}
}

// Response wraps http.Response with convenience helpers.
type Response struct {
	*http.Response
}

// BodyBytes reads and returns the entire response body.
func (r *Response) BodyBytes() ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(r.Body)
}

// IsSuccess returns true if the status code is 2xx.
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsRetryable returns true if the response indicates a retryable error.
func (r *Response) IsRetryable() bool {
	switch r.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusBadGateway:
		return true
	}
	return false
}

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	// CircuitClosed means requests are allowed.
	CircuitClosed CircuitState = iota
	// CircuitOpen means requests are blocked.
	CircuitOpen
	// CircuitHalfOpen means a single test request is allowed.
	CircuitHalfOpen
)

// circuitBreaker implements the circuit breaker pattern.
type circuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failures        int
	threshold       int
	timeout         time.Duration
	lastFailure     time.Time
	lastStateChange time.Time
}

// newCircuitBreaker creates a new circuit breaker.
func newCircuitBreaker(threshold int, timeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		state:     CircuitClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

// allow checks if a request should be allowed.
func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastStateChange) > cb.timeout {
			cb.state = CircuitHalfOpen
			cb.lastStateChange = time.Now()
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return false
}

// recordSuccess records a successful request.
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		cb.lastStateChange = time.Now()
	}
}

// recordFailure records a failed request.
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
		cb.lastStateChange = time.Now()
	}
}

// State returns the current circuit state.
func (cb *circuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Client is an HTTP client with retry, timeout, and circuit breaker.
type Client struct {
	config  *Config
	client  *http.Client
	breaker *circuitBreaker
}

// NewClient creates a new Client. If config is nil, DefaultConfig is used.
func NewClient(config *Config) *Client {
	if config == nil {
		config = DefaultConfig()
	}

	return &Client{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		breaker: newCircuitBreaker(
			config.CircuitBreakerThreshold,
			config.CircuitBreakerTimeout,
		),
	}
}

// Request sends an HTTP request with automatic retry and circuit breaker.
func (c *Client) Request(
	ctx context.Context,
	method, url string,
	body io.Reader,
) (*Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf(
					"context cancelled during retry: %w", ctx.Err(),
				)
			case <-time.After(backoff):
			}
		}

		if !c.breaker.allow() {
			lastErr = fmt.Errorf("circuit breaker is open")
			continue
		}

		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			c.breaker.recordFailure()
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		response := &Response{Response: resp}

		if response.IsSuccess() {
			c.breaker.recordSuccess()
			return response, nil
		}

		if response.IsRetryable() {
			c.breaker.recordFailure()
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("retryable HTTP %d", resp.StatusCode)
			continue
		}

		// Non-retryable error, return immediately
		c.breaker.recordSuccess() // not a server issue
		return response, nil
	}

	return nil, fmt.Errorf(
		"request failed after %d attempts: %w",
		c.config.MaxRetries+1, lastErr,
	)
}

// calculateBackoff computes exponential backoff.
func (c *Client) calculateBackoff(attempt int) time.Duration {
	backoff := time.Duration(
		float64(c.config.BackoffBase) * math.Pow(2, float64(attempt-1)),
	)
	if backoff > c.config.BackoffMax {
		backoff = c.config.BackoffMax
	}
	return backoff
}

// CircuitState returns the current circuit breaker state.
func (c *Client) CircuitState() CircuitState {
	return c.breaker.State()
}
