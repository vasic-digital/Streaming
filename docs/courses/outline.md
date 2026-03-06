# Course: Streaming and Transport Abstractions in Go

## Module Overview

This course covers the `digital.vasic.streaming` module, teaching Server-Sent Events, WebSocket communication with rooms, gRPC streaming, webhook dispatch with signing, resilient HTTP clients, transport abstractions, and real-time file change notification. Each lesson builds on Go concurrency patterns and standard library idioms.

## Prerequisites

- Intermediate Go knowledge (goroutines, channels, `net/http`, context)
- Basic understanding of WebSocket, SSE, and gRPC concepts
- Go 1.24+ installed

## Lessons

| # | Title | Duration |
|---|-------|----------|
| 1 | Server-Sent Events and WebSocket Hub | 45 min |
| 2 | gRPC Streaming and Webhook Dispatch | 45 min |
| 3 | HTTP Client, Transport Abstraction, and Real-Time Events | 40 min |

## Source Files

- `pkg/sse/` -- SSE broker with heartbeat and client management
- `pkg/websocket/` -- WebSocket hub with rooms and ping/pong
- `pkg/grpc/` -- gRPC streaming, interceptors, and health server
- `pkg/webhook/` -- Webhook dispatch, signing, and registry
- `pkg/http/` -- HTTP client with circuit breaker
- `pkg/transport/` -- Transport abstraction layer
- `pkg/realtime/` -- Debouncer and path filter
- `pkg/gin/` -- Gin framework adapters
