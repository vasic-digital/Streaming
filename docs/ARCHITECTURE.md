# Architecture

## Overview

`digital.vasic.streaming` is a Go module providing six independent packages for streaming and transport. The module is designed as a library -- it does not run as a standalone service. Consumers import individual packages as needed.

The module follows three guiding principles:

1. **Independence** -- Each package is self-contained with no internal cross-dependencies.
2. **Concurrency safety** -- All exported types are safe for concurrent use.
3. **Sensible defaults** -- Every package provides a `DefaultConfig()` and nil-safe constructors.

## Package Architecture

```
digital.vasic.streaming/
  pkg/
    sse/          SSE broker (Observer pattern)
    websocket/    WebSocket hub (Hub + Pub/Sub)
    grpc/         gRPC utilities (Adapter + Chain of Responsibility)
    webhook/      Webhook dispatcher (Strategy + Retry)
    http/         HTTP client (Circuit Breaker + Retry)
    transport/    Transport abstraction (Strategy + Factory)
```

### Dependency Graph

The six packages have zero internal dependencies. Each package depends only on the Go standard library and its specific external dependency (if any):

| Package | External Dependencies |
|---------|----------------------|
| `pkg/sse` | None (stdlib only) |
| `pkg/websocket` | `github.com/gorilla/websocket` |
| `pkg/grpc` | `google.golang.org/grpc` |
| `pkg/webhook` | None (stdlib only) |
| `pkg/http` | None (stdlib only) |
| `pkg/transport` | None (stdlib only) |

This zero-coupling design means consumers can import any single package without pulling in unrelated dependencies.

## Design Patterns

### Observer Pattern -- SSE Broker

The `sse.Broker` implements the Observer pattern. Clients register (subscribe) via `AddClient`, and the broker broadcasts events to all registered observers.

```
Broker (Subject)
  |-- AddClient() -> registers observer
  |-- RemoveClient() -> unregisters observer
  |-- Broadcast() -> notifies all observers
  |-- SendTo() -> notifies specific observer
```

Key design decisions:
- **Non-blocking broadcast**: If a client's channel buffer is full, the event is dropped for that client rather than blocking the broadcast. This prevents a slow consumer from affecting other clients.
- **Automatic cleanup**: Clients are removed when their HTTP request context is cancelled (browser disconnect).
- **Heartbeat goroutine**: A background goroutine sends periodic heartbeat events to keep connections alive and detect dead clients.

### Hub Pattern -- WebSocket

The `websocket.Hub` manages client lifecycle through register/unregister channels, processed in a single goroutine (`run`). This avoids complex locking for the registration flow.

```
Hub
  |-- register channel <- Client
  |-- unregister channel <- Client
  |-- run() goroutine processes both channels
  |
  |-- Room (group of clients)
       |-- clients map
       |-- SendToRoom() broadcasts within room
```

Key design decisions:
- **Channel-based registration**: The `register` and `unregister` channels are buffered (256) to prevent blocking during high-connection-rate scenarios.
- **Per-client goroutine pair**: Each client gets a `readPump` and `writePump` goroutine. The read pump deserializes JSON messages and dispatches to the `onMessage` handler. The write pump serializes outgoing messages and manages ping/pong.
- **Room auto-creation**: Rooms are created lazily on first `JoinRoom` call. Clients are removed from all rooms on disconnect.
- **Origin checking**: The upgrader validates the `Origin` header against `AllowedOrigins`. A wildcard `*` allows all origins.

### Strategy Pattern -- Transport Abstraction

The `transport.Transport` interface defines the strategy contract (Send, Receive, Close). Concrete implementations (channel-based for now, with HTTP/WebSocket/gRPC planned) provide protocol-specific behavior.

```
Transport (Strategy Interface)
  |-- Send(ctx, data) error
  |-- Receive(ctx) (data, error)
  |-- Close() error

Factory (Strategy Creator)
  |-- Register(type, creator)
  |-- Create(config) -> Transport
```

Key design decisions:
- **Factory pattern**: `transport.NewFactory()` registers built-in creators. Consumers can register custom transport types via `Register`.
- **Channel-based default**: The built-in `channelTransport` uses Go channels for in-process communication. This serves as both the default implementation and a testing tool.
- **Context-aware operations**: Both `Send` and `Receive` accept `context.Context` for cancellation and timeout propagation.

### Adapter Pattern -- gRPC Utilities

The `grpc` package adapts the standard `google.golang.org/grpc` library with higher-level abstractions:

- `StreamServer` interface simplifies the four gRPC streaming patterns into channel-based APIs.
- `StreamInterceptor` type wraps `grpc.StreamServerInterceptor` with a simpler signature.
- `HealthServer` wraps `grpc_health_v1` with a mutable service status map.

### Chain of Responsibility -- Interceptor Chaining

`ChainStreamInterceptors` composes multiple `StreamInterceptor` functions into a single interceptor. Each interceptor in the chain can:
- Modify the context or stream before calling the next handler
- Inspect the result after the next handler returns
- Short-circuit the chain by returning an error

```
Request -> Interceptor1 -> Interceptor2 -> ... -> Handler -> Response
```

The chain is built in reverse order so that the first interceptor in the variadic argument list executes first.

### Circuit Breaker -- HTTP Client

The `http.Client` implements the Circuit Breaker pattern to prevent cascading failures:

```
States:
  Closed ----[failures >= threshold]----> Open
  Open   ----[timeout elapsed]----------> HalfOpen
  HalfOpen --[success]-------------------> Closed
  HalfOpen --[failure]-------------------> Open
```

Key design decisions:
- **Integrated with retry**: The circuit breaker is checked on each retry attempt. If the circuit is open, the attempt is skipped (counted as a failure for retry purposes).
- **Non-retryable responses pass through**: 4xx errors (except 429) are treated as non-retryable and returned immediately. The circuit breaker records a success for these since they indicate the server is reachable.
- **Retryable status codes**: 429 (Too Many Requests), 502 (Bad Gateway), 503 (Service Unavailable), 504 (Gateway Timeout).

### Retry with Exponential Backoff

Both `http.Client` and `webhook.Dispatcher` implement exponential backoff:

```
backoff = BackoffBase * 2^(attempt-1)
if backoff > BackoffMax:
    backoff = BackoffMax
```

The backoff is capped at `BackoffMax` to prevent excessively long waits. Context cancellation is checked between retry attempts.

## Concurrency Model

### SSE Broker
- `sync.RWMutex` protects the clients map (read-heavy: broadcasts read, add/remove write).
- `atomic.Int64` for client count to avoid locking on `ClientCount()`.
- One background goroutine for heartbeats, tracked by `sync.WaitGroup`.

### WebSocket Hub
- `sync.RWMutex` for clients map and rooms map (separate mutexes to reduce contention).
- Buffered channels (256) for register/unregister to prevent blocking.
- One goroutine for the hub run loop, plus two goroutines per client (read pump, write pump).
- `sync.Mutex` on each `Client` for send buffer and close coordination.

### gRPC HealthServer
- `sync.RWMutex` protects the services status map.

### Webhook Dispatcher
- `sync.RWMutex` protects delivery statistics counters.
- No goroutine management -- callers control concurrency.

### Webhook Registry
- `sync.RWMutex` protects the webhooks map.
- `List()` returns a copy of the map to prevent external mutation.

### HTTP Client
- `sync.RWMutex` in the circuit breaker for state transitions.
- No goroutine management -- callers control concurrency.

### Transport
- `sync.RWMutex` in the Factory for creator registration.
- `sync.Mutex` in channelTransport for close coordination.

## Configuration Pattern

Every package follows the same configuration contract:

```go
// 1. Config struct with documented fields
type Config struct {
    FieldA int
    FieldB time.Duration
}

// 2. DefaultConfig with sensible production defaults
func DefaultConfig() *Config {
    return &Config{
        FieldA: 100,
        FieldB: 30 * time.Second,
    }
}

// 3. Constructor accepts *Config, nil falls back to defaults
func NewThing(config *Config) *Thing {
    if config == nil {
        config = DefaultConfig()
    }
    // ...
}
```

This pattern ensures:
- Zero-config usage works out of the box
- All defaults are documented in one place
- Partial configuration is not possible (you either override everything or use defaults)

## Error Handling

All errors are wrapped with `fmt.Errorf("operation description: %w", err)` to preserve the error chain. Sentinel errors are not used -- callers should check behavior (e.g., `errors.Is(err, context.Canceled)`) rather than matching specific error strings.

Functions that can fail for predictable reasons (client not found, transport closed) return `error`. Functions that manage lifecycle (Close, RemoveClient) are idempotent and safe to call multiple times.

## Graceful Shutdown

All long-lived types (`Broker`, `Hub`) support graceful shutdown:

1. `Close()` cancels the internal context
2. Background goroutines detect cancellation and exit
3. `sync.WaitGroup.Wait()` blocks until all goroutines have stopped
4. Client connections are closed and maps are cleared
