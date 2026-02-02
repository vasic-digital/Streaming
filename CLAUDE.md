# CLAUDE.md - Streaming Module

## Overview

`digital.vasic.streaming` is a generic, reusable Go module for streaming and transport abstractions. It provides Server-Sent Events (SSE), WebSocket, gRPC streaming utilities, webhook dispatch, HTTP client utilities, and a unified transport abstraction layer.

**Module**: `digital.vasic.streaming` (Go 1.24+)

## Build & Test

```bash
go build ./...
go test ./... -count=1 -race
go test ./... -short              # Unit tests only
go test -tags=integration ./...   # Integration tests
go test -bench=. ./...            # Benchmarks
```

## Code Style

- Standard Go conventions, `gofmt` formatting
- Imports grouped: stdlib, third-party, internal (blank line separated)
- Line length <= 100 chars
- Naming: `camelCase` private, `PascalCase` exported, acronyms all-caps
- Errors: always check, wrap with `fmt.Errorf("...: %w", err)`
- Tests: table-driven, `testify`, naming `Test<Struct>_<Method>_<Scenario>`

## Package Structure

| Package | Purpose |
|---------|---------|
| `pkg/sse` | Server-Sent Events broker with heartbeat, reconnection |
| `pkg/websocket` | WebSocket hub with rooms, read/write pumps, ping/pong |
| `pkg/grpc` | gRPC streaming utilities, interceptors, health server |
| `pkg/webhook` | Webhook dispatch with retry, HMAC-SHA256 signing |
| `pkg/http` | HTTP client with retry, timeout, circuit breaker |
| `pkg/transport` | Transport abstraction layer (HTTP, WebSocket, gRPC) |

## Key Interfaces

- `transport.Transport` -- Send, Receive, Close
- `grpc.StreamServer` -- Unary, server-stream, client-stream, bidirectional
- `grpc.StreamInterceptor` -- Middleware on gRPC streams
- `webhook.Registry` -- Webhook subscription management

## Design Patterns

- **Strategy**: Transport interface (HTTP/WebSocket/gRPC)
- **Factory**: `transport.NewFactory()` creates transports by type
- **Observer**: SSE Broker broadcasts to connected clients
- **Hub**: WebSocket Hub manages connections and rooms
- **Circuit Breaker**: HTTP client with configurable failure thresholds
- **Retry with Backoff**: Webhook dispatcher and HTTP client

## Commit Style

Conventional Commits: `feat(sse): add reconnection support`
