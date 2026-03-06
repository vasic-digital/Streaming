# Streaming Module

`digital.vasic.streaming` is a generic, reusable Go module for streaming and transport abstractions. It provides Server-Sent Events (SSE), WebSocket with rooms, gRPC streaming utilities, webhook dispatch with HMAC signing, an HTTP client with circuit breaker, a unified transport abstraction layer, real-time file change notification with debouncing, and Gin framework adapters.

## Key Features

- **SSE broker** -- Server-Sent Events with heartbeat, reconnection tracking, max client limits, and per-client targeting
- **WebSocket hub** -- Room-based messaging with ping/pong, read/write pumps, and origin checking
- **gRPC streaming** -- Server interfaces for unary, server-stream, client-stream, and bidirectional RPCs with interceptor chaining and health checks
- **Webhook dispatch** -- HMAC-SHA256 signed delivery with retry, exponential backoff, and a subscription registry
- **HTTP client** -- Automatic retry with exponential backoff and circuit breaker (closed/open/half-open states)
- **Transport abstraction** -- Unified Send/Receive/Close interface with factory for HTTP, WebSocket, and gRPC transports
- **Real-time events** -- File change debouncing with batched delivery and path filtering (include/exclude globs)
- **Gin adapters** -- SSE and WebSocket handlers for the Gin framework

## Package Overview

| Package | Purpose |
|---------|---------|
| `pkg/sse` | Server-Sent Events broker with heartbeat |
| `pkg/websocket` | WebSocket hub with rooms and ping/pong |
| `pkg/grpc` | gRPC streaming, interceptors, and health server |
| `pkg/webhook` | Webhook dispatch with HMAC signing and retry |
| `pkg/http` | HTTP client with circuit breaker and retry |
| `pkg/transport` | Transport abstraction layer (HTTP, WebSocket, gRPC) |
| `pkg/realtime` | File change debouncing and path filtering |
| `pkg/gin` | Gin framework adapters for SSE and WebSocket |

## Installation

```bash
go get digital.vasic.streaming
```

Requires Go 1.24 or later.
