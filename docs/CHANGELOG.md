# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-01-01

### Added

#### Package `sse`
- `Event` struct with `Format()` method for SSE wire format serialization.
- `Client` struct representing a connected SSE client with ID, channel, and reconnection tracking.
- `Config` struct with `BufferSize`, `HeartbeatInterval`, and `MaxClients` fields.
- `DefaultConfig()` returning sensible defaults (100 buffer, 30s heartbeat, 1000 max clients).
- `Broker` for managing SSE client connections with `AddClient`, `RemoveClient`, `Broadcast`, `SendTo`, `ClientCount`, and `Close` methods.
- `http.Handler` implementation on `Broker` via `ServeHTTP`.
- Automatic heartbeat goroutine sending periodic heartbeat events.
- `X-Client-ID` header support for client identification.
- `Last-Event-ID` header support for reconnection tracking.

#### Package `websocket`
- `Message` struct with JSON serialization for type, room, data, and sender fields.
- `Config` struct with read/write buffer sizes, ping/pong/write timeouts, max message size, and allowed origins.
- `DefaultConfig()` returning sensible defaults (1024 buffers, 54s ping, 60s pong, 512KB max).
- `Client` with `ID()`, `Send()`, and `Close()` methods.
- `Room` for grouping clients.
- `Hub` for managing WebSocket connections with `ServeWS`, `Broadcast`, `SendToRoom`, `JoinRoom`, `LeaveRoom`, `ClientCount`, `RoomCount`, `RoomClientCount`, `OnMessage`, and `Close` methods.
- Read pump with pong handler and read limit enforcement.
- Write pump with ping interval and write deadline management.
- Origin validation via `AllowedOrigins` configuration.
- `X-Client-ID` header support for custom client IDs.

#### Package `grpc`
- `Config` struct with address, TLS, message size, and concurrent stream settings.
- `DefaultConfig()` returning sensible defaults (:50051, 4MB messages, 100 streams).
- `StreamServer` interface defining Unary, ServerStream, ClientStream, and BidirectionalStream patterns.
- `StreamInterceptor` function type for gRPC stream middleware.
- `ChainStreamInterceptors` for composing multiple interceptors.
- `MetadataInterceptor` for extracting gRPC metadata into context.
- `LoggingInterceptor` for logging stream method calls and errors.
- `HealthServer` implementing the gRPC health check protocol with `SetServiceStatus`, `Check`, and `Watch`.
- `RegisterHealthServer` helper for registering with a gRPC server.
- `ServerOptions` helper for deriving gRPC server options from Config.

#### Package `webhook`
- `Webhook` struct representing a webhook subscription with URL, secret, events, and active status.
- `Payload` struct with ID, event, data, timestamp, and signature fields.
- `DispatcherConfig` with max retries, timeout, backoff, signature header, and user agent settings.
- `DefaultDispatcherConfig()` returning sensible defaults (5 retries, 30s timeout, 1s backoff).
- `Sign` and `Verify` functions for HMAC-SHA256 payload signing and verification.
- `Dispatcher` with `Send` (retry + backoff) and `Stats` methods.
- `Registry` for managing webhook subscriptions with `Register`, `Unregister`, `Get`, `List`, `Count`, and `MatchingWebhooks` methods.
- Wildcard event matching support.

#### Package `http`
- `Config` struct with retry, timeout, backoff, and circuit breaker settings.
- `DefaultConfig()` returning sensible defaults (3 retries, 30s timeout, 5 failure threshold).
- `Response` wrapper with `BodyBytes()`, `IsSuccess()`, and `IsRetryable()` helpers.
- `CircuitState` type with `CircuitClosed`, `CircuitOpen`, and `CircuitHalfOpen` constants.
- `Client` with `Request` (retry + circuit breaker) and `CircuitState` methods.
- Exponential backoff with configurable base and maximum.
- Circuit breaker with Closed/Open/HalfOpen state transitions.
- Retryable status code detection (429, 502, 503, 504).

#### Package `transport`
- `Type` constants for HTTP, WebSocket, and gRPC transports.
- `Config` struct with type, address, and options fields.
- `Transport` interface with `Send`, `Receive`, and `Close` methods.
- `Factory` with `Register`, `Create`, and `SupportedTypes` methods.
- `NewFactory()` pre-registered with built-in channel-based transports.
- Channel-based transport implementation for in-process communication and testing.

#### Documentation
- `CLAUDE.md` with module overview, build/test commands, and design patterns.
- `README.md` with installation and quick start.
- `AGENTS.md` multi-agent coordination guide.
- `docs/USER_GUIDE.md` with code examples for all six packages.
- `docs/ARCHITECTURE.md` with design decisions and patterns.
- `docs/API_REFERENCE.md` with complete exported API documentation.
- `docs/CONTRIBUTING.md` with development workflow and conventions.
- `docs/diagrams/` with Mermaid architecture, sequence, and class diagrams.
