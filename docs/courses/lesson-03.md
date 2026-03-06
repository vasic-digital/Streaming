# Lesson 3: HTTP Client, Transport Abstraction, and Real-Time Events

## Learning Objectives

- Build resilient HTTP requests with retry and circuit breaker
- Use the transport factory to create protocol-agnostic communication channels
- Debounce file change events and filter paths with glob patterns

## Key Concepts

- **Circuit Breaker States**: `Closed` (normal operation, counting failures), `Open` (requests blocked after threshold failures), `HalfOpen` (one test request allowed after timeout). Successful requests in half-open state close the circuit.
- **Transport Interface**: `Transport` defines `Send(ctx, data) error`, `Receive(ctx) (data, error)`, and `Close() error`. The `Factory` creates transports by type. Built-in implementations use channels; real implementations can be registered.
- **Debouncer**: Aggregates `ChangeEvent` values over a configurable time window. The timer resets on each new event. When the window expires, all pending events are flushed to the handler as a single batch.
- **PathFilter**: Uses `filepath.Match` glob syntax. Excludes take priority over includes. Both full path and base name are checked for each pattern.

## Code Walkthrough

### Source: `pkg/http/http.go`

The `Client.Request` method loops up to `MaxRetries`, checking the circuit breaker before each attempt. Successful responses record success (closing the circuit if half-open). Retryable status codes (429, 502, 503, 504) record failure and continue.

### Source: `pkg/transport/transport.go`

`Factory.Create(config)` looks up a creator function by transport type and returns a `Transport` instance. Custom creators are registered with `Factory.Register(type, creator)`.

### Source: `pkg/realtime/realtime.go`

`Debouncer.Notify(event)` appends the event to the pending slice and resets the timer. The `run()` goroutine waits on `flushCh` or context cancellation. `Stop()` cancels the context and waits for the goroutine to drain remaining events.

### Source: `pkg/realtime/filter.go`

`PathFilter.Match(path)` checks excludes first (priority over includes), then checks includes. Empty includes means all paths are accepted.

## Practice Exercise

1. Create an HTTP client with a circuit breaker threshold of 3. Send requests to a failing server and verify the circuit opens after 3 failures. Wait for the timeout and verify half-open allows one test request.
2. Create a transport factory, register a custom transport type "custom", create an instance, and verify Send/Receive works through it.
3. Create a debouncer with a 200ms window. Send 10 events in rapid succession and verify they arrive as a single batch. Create a path filter and verify include/exclude behavior.
