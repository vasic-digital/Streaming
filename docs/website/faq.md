# FAQ

## What happens when an SSE client's buffer is full?

When broadcasting, if a client's channel buffer is full, the event is silently dropped for that client. This prevents a slow client from blocking delivery to other clients. The buffer size is configurable via `Config.BufferSize` (default 100).

## How does the circuit breaker work in the HTTP client?

The circuit breaker has three states: `Closed` (requests allowed, counting failures), `Open` (requests blocked), and `HalfOpen` (one test request allowed). After `CircuitBreakerThreshold` consecutive failures, the circuit opens. After `CircuitBreakerTimeout`, it transitions to half-open. A successful request in half-open state closes the circuit.

## Is the WebSocket hub thread-safe?

Yes. The hub uses a combination of channels (for register/unregister) and `sync.RWMutex` (for client and room access) to ensure thread safety. The hub's `run()` goroutine processes registration events sequentially, while broadcast operations use read locks.

## How does the debouncer handle rapid events?

The debouncer resets its timer on every new event. Events accumulate in a pending slice. When the debounce window expires without new events, the entire batch is flushed to the handler in a single call. This coalesces rapid changes into fewer, larger batches.

## Can I use the transport abstraction for real network communication?

The built-in transport implementations use Go channels for in-process communication and testing. For real network protocols, register custom creators with `Factory.Register()` that return Transport implementations backed by actual HTTP, WebSocket, or gRPC connections.
