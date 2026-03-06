# Lesson 4: Transport Abstraction, Realtime Filters, and Gin Adapter

## Learning Objectives

- Design a transport abstraction layer with the Strategy and Factory patterns
- Build a realtime event system with configurable filters
- Integrate streaming capabilities with the Gin web framework

## Key Concepts

- **Strategy Interface (Transport)**: `Transport` defines `Send(ctx, data)`, `Receive(ctx)`, and `Close()`. Concrete implementations provide protocol-specific behavior. The built-in `channelTransport` uses Go channels for in-process communication.
- **Factory Pattern**: `transport.NewFactory()` registers built-in transport creators. Consumers can register custom types via `Register`. `Create(config)` produces the appropriate transport based on configuration.
- **Realtime Filters**: The `realtime` package provides configurable event filtering with matcher functions, enabling clients to subscribe to specific event types or topics.
- **Default Config Pattern**: Every package provides `DefaultConfig()` and nil-safe constructors, ensuring zero-config usage works out of the box.

## Code Walkthrough

### Source: `pkg/transport/transport.go`

The transport interface and factory:

```go
type Transport interface {
    Send(ctx context.Context, data []byte) error
    Receive(ctx context.Context) ([]byte, error)
    Close() error
}
```

The factory uses `sync.RWMutex` for thread-safe creator registration. The channel-based default uses `sync.Mutex` for close coordination.

### Source: `pkg/realtime/realtime.go` and `pkg/realtime/filter.go`

The realtime package provides an event system with subscription-based delivery. Filters allow clients to receive only events matching their criteria (by type, topic, or custom predicates).

### Source: `pkg/gin/gin.go`

The Gin adapter integrates streaming endpoints (SSE, WebSocket) with Gin routers, handling the HTTP upgrade and context propagation.

### Tests

Tests verify transport send/receive, factory registration, filter matching, and Gin integration.

## Practice Exercise

1. Create a channel-based transport, send 5 messages, and receive them. Verify all messages arrive in order and the transport closes cleanly.
2. Register a custom transport type called "mock" with the factory. Create an instance and verify it works through the factory's `Create` method.
3. Set up a realtime event system with two subscribers: one filtered by event type "file_created" and another unfiltered. Publish events of different types and verify each subscriber receives only the correct events.
