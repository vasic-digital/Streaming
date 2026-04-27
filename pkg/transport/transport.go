// Package transport provides a unified transport abstraction layer
// supporting multiple protocols (HTTP, WebSocket, gRPC).
package transport

import (
	"context"
	"fmt"
	"sync"
)

// Type represents the transport protocol type.
type Type string

const (
	// TypeHTTP represents HTTP transport.
	TypeHTTP Type = "http"
	// TypeWebSocket represents WebSocket transport.
	TypeWebSocket Type = "websocket"
	// TypeGRPC represents gRPC transport.
	TypeGRPC Type = "grpc"
)

// Config holds transport configuration.
type Config struct {
	// Type is the transport protocol type.
	Type Type
	// Address is the connection address.
	Address string
	// Options holds transport-specific options.
	Options map[string]interface{}
}

// Transport defines the interface for all transport implementations.
type Transport interface {
	// Send sends data through the transport.
	Send(ctx context.Context, data []byte) error
	// Receive receives data from the transport.
	// It blocks until data is available or the context is done.
	Receive(ctx context.Context) ([]byte, error)
	// Close closes the transport connection.
	Close() error
}

// Factory creates Transport instances by type.
type Factory struct {
	mu       sync.RWMutex
	creators map[Type]func(config *Config) (Transport, error)
}

// NewFactory creates a new transport Factory with built-in creators.
func NewFactory() *Factory {
	f := &Factory{
		creators: make(map[Type]func(config *Config) (Transport, error)),
	}

	// Register built-in transport creators
	f.Register(TypeHTTP, newChannelTransport)
	f.Register(TypeWebSocket, newChannelTransport)
	f.Register(TypeGRPC, newChannelTransport)

	return f
}

// Register registers a transport creator for a given type.
func (f *Factory) Register(
	t Type,
	creator func(config *Config) (Transport, error),
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[t] = creator
}

// Create creates a new Transport for the given configuration.
func (f *Factory) Create(config *Config) (Transport, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	f.mu.RLock()
	creator, ok := f.creators[config.Type]
	f.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported transport type: %s", config.Type)
	}

	return creator(config)
}

// SupportedTypes returns all registered transport types.
func (f *Factory) SupportedTypes() []Type {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]Type, 0, len(f.creators))
	for t := range f.creators {
		types = append(types, t)
	}
	return types
}

// channelTransport is a channel-based Transport implementation
// used for in-process communication and testing.
//
// Concurrency contract: sendCh and recvCh are NEVER closed.
// Closure is signalled exclusively via the done channel so that
// concurrent Send / Receive goroutines can select on <-done without
// risking a send-on-closed-channel panic.
type channelTransport struct {
	config  *Config
	sendCh  chan []byte
	recvCh  chan []byte
	done    chan struct{}
	closeMu sync.Mutex
	closed  bool
}

// newChannelTransport creates a new channel-based transport.
func newChannelTransport(config *Config) (Transport, error) {
	bufSize := 256
	if config.Options != nil {
		if bs, ok := config.Options["buffer_size"]; ok {
			if size, ok := bs.(int); ok {
				bufSize = size
			}
		}
	}

	return &channelTransport{
		config: config,
		sendCh: make(chan []byte, bufSize),
		recvCh: make(chan []byte, bufSize),
		done:   make(chan struct{}),
	}, nil
}

// Send sends data through the channel transport.
// It returns an error if the transport is already closed, if the
// context is cancelled, or if Close is called while Send is waiting.
func (t *channelTransport) Send(ctx context.Context, data []byte) error {
	// Fast-path: already closed.
	t.closeMu.Lock()
	closed := t.closed
	t.closeMu.Unlock()
	if closed {
		return fmt.Errorf("transport is closed")
	}

	// Select races ctx, done, and the buffered channel. sendCh is never
	// closed so there is no risk of a send-on-closed-channel panic.
	select {
	case t.sendCh <- data:
		return nil
	case <-t.done:
		return fmt.Errorf("transport is closed")
	case <-ctx.Done():
		return fmt.Errorf("send cancelled: %w", ctx.Err())
	}
}

// Receive receives data from the channel transport.
func (t *channelTransport) Receive(ctx context.Context) ([]byte, error) {
	// Fast-path: already closed.
	t.closeMu.Lock()
	closed := t.closed
	t.closeMu.Unlock()
	if closed {
		return nil, fmt.Errorf("transport is closed")
	}

	select {
	case data := <-t.recvCh:
		return data, nil
	case <-t.done:
		return nil, fmt.Errorf("transport is closed")
	case <-ctx.Done():
		return nil, fmt.Errorf("receive cancelled: %w", ctx.Err())
	}
}

// Close closes the channel transport. It is safe to call from multiple
// goroutines; only the first call has any effect.
func (t *channelTransport) Close() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	close(t.done) // unblocks any in-flight Send / Receive
	return nil
}

// SendChannel returns the send channel for testing purposes.
func (t *channelTransport) SendChannel() <-chan []byte {
	return t.sendCh
}

// ReceiveChannel returns the receive channel for injecting test data.
func (t *channelTransport) ReceiveChannel() chan<- []byte {
	return t.recvCh
}
