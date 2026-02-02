// Package webhook provides webhook dispatch with retry, backoff,
// HMAC-SHA256 signing, and a subscription registry.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

// Webhook represents a webhook subscription.
type Webhook struct {
	// URL is the endpoint to deliver events to.
	URL string `json:"url"`
	// Secret is the HMAC signing key.
	Secret string `json:"secret,omitempty"`
	// Events is the list of event types to subscribe to.
	// Empty means all events.
	Events []string `json:"events,omitempty"`
	// Active indicates whether this webhook is enabled.
	Active bool `json:"active"`
}

// Payload represents a webhook delivery payload.
type Payload struct {
	// ID is the unique delivery identifier.
	ID string `json:"id"`
	// Event is the event type.
	Event string `json:"event"`
	// Data is the event payload.
	Data interface{} `json:"data"`
	// Timestamp is when the event was generated.
	Timestamp time.Time `json:"timestamp"`
	// Signature is the HMAC-SHA256 signature (set after signing).
	Signature string `json:"signature,omitempty"`
}

// DispatcherConfig holds configuration for the Dispatcher.
type DispatcherConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// Timeout is the HTTP request timeout.
	Timeout time.Duration
	// BackoffBase is the base duration for exponential backoff.
	BackoffBase time.Duration
	// BackoffMax is the maximum backoff duration.
	BackoffMax time.Duration
	// SignatureHeader is the HTTP header name for the signature.
	SignatureHeader string
	// UserAgent is the User-Agent header value.
	UserAgent string
}

// DefaultDispatcherConfig returns a DispatcherConfig with sensible defaults.
func DefaultDispatcherConfig() *DispatcherConfig {
	return &DispatcherConfig{
		MaxRetries:      5,
		Timeout:         30 * time.Second,
		BackoffBase:     time.Second,
		BackoffMax:      5 * time.Minute,
		SignatureHeader: "X-Signature-256",
		UserAgent:       "Streaming-Webhook/1.0",
	}
}

// Sign computes the HMAC-SHA256 signature for a payload using the secret.
func Sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks that the signature matches the payload and secret.
func Verify(payload []byte, signature, secret string) bool {
	expected := Sign(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// Dispatcher handles webhook delivery with retry logic.
type Dispatcher struct {
	config *DispatcherConfig
	client *http.Client

	mu             sync.RWMutex
	deliveredCount int64
	failedCount    int64
}

// NewDispatcher creates a new Dispatcher. If config is nil,
// DefaultDispatcherConfig is used.
func NewDispatcher(config *DispatcherConfig) *Dispatcher {
	if config == nil {
		config = DefaultDispatcherConfig()
	}

	return &Dispatcher{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Send delivers a payload to a webhook with retry and backoff.
func (d *Dispatcher) Send(
	ctx context.Context,
	webhook *Webhook,
	payload *Payload,
) error {
	if !webhook.Active {
		return fmt.Errorf("webhook is not active")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Sign if secret is configured
	if webhook.Secret != "" {
		payload.Signature = Sign(payloadBytes, webhook.Secret)
	}

	var lastErr error
	for attempt := 0; attempt <= d.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := d.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		lastErr = d.doSend(ctx, webhook, payloadBytes)
		if lastErr == nil {
			d.mu.Lock()
			d.deliveredCount++
			d.mu.Unlock()
			return nil
		}
	}

	d.mu.Lock()
	d.failedCount++
	d.mu.Unlock()

	return fmt.Errorf(
		"webhook delivery failed after %d attempts: %w",
		d.config.MaxRetries+1,
		lastErr,
	)
}

// doSend performs a single HTTP delivery attempt.
func (d *Dispatcher) doSend(
	ctx context.Context,
	webhook *Webhook,
	payload []byte,
) error {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, webhook.URL, bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", d.config.UserAgent)

	if webhook.Secret != "" {
		signature := Sign(payload, webhook.Secret)
		req.Header.Set(d.config.SignatureHeader, signature)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read and discard body
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
}

// calculateBackoff calculates exponential backoff for retries.
func (d *Dispatcher) calculateBackoff(attempt int) time.Duration {
	backoff := time.Duration(
		float64(d.config.BackoffBase) * math.Pow(2, float64(attempt-1)),
	)
	if backoff > d.config.BackoffMax {
		backoff = d.config.BackoffMax
	}
	return backoff
}

// Stats returns delivery statistics.
func (d *Dispatcher) Stats() (delivered, failed int64) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.deliveredCount, d.failedCount
}

// Registry manages webhook subscriptions.
type Registry struct {
	mu       sync.RWMutex
	webhooks map[string]*Webhook
}

// NewRegistry creates a new webhook Registry.
func NewRegistry() *Registry {
	return &Registry{
		webhooks: make(map[string]*Webhook),
	}
}

// Register adds a webhook subscription with the given ID.
func (r *Registry) Register(id string, webhook *Webhook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.webhooks[id] = webhook
}

// Unregister removes a webhook subscription.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.webhooks, id)
}

// Get returns a webhook by ID.
func (r *Registry) Get(id string) (*Webhook, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	wh, ok := r.webhooks[id]
	return wh, ok
}

// List returns all registered webhooks.
func (r *Registry) List() map[string]*Webhook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*Webhook, len(r.webhooks))
	for k, v := range r.webhooks {
		result[k] = v
	}
	return result
}

// Count returns the number of registered webhooks.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.webhooks)
}

// MatchingWebhooks returns all active webhooks subscribed to the event.
func (r *Registry) MatchingWebhooks(event string) []*Webhook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*Webhook
	for _, wh := range r.webhooks {
		if !wh.Active {
			continue
		}
		if len(wh.Events) == 0 {
			matched = append(matched, wh)
			continue
		}
		for _, e := range wh.Events {
			if e == event || e == "*" {
				matched = append(matched, wh)
				break
			}
		}
	}
	return matched
}
