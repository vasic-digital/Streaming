# AGENTS.md - Multi-Agent Coordination Guide

## Overview

This document provides guidance for AI agents (Claude Code, Copilot, etc.) working with the `digital.vasic.streaming` module. It describes how multiple agents should coordinate when making changes across the six packages in this module.

## Module Structure

```
Streaming/
  pkg/
    sse/          - Server-Sent Events broker
    websocket/    - WebSocket hub with rooms
    grpc/         - gRPC streaming utilities
    webhook/      - Webhook dispatch with retry
    http/         - HTTP client with circuit breaker
    transport/    - Unified transport abstraction
```

## Package Ownership and Dependencies

The packages form a layered dependency graph. Agents must understand which packages are independent and which share contracts.

### Independent Packages (no internal cross-dependencies)

These packages can be modified in parallel by separate agents without coordination:

- **pkg/sse** - Standalone SSE broker. Depends only on `net/http` from stdlib.
- **pkg/websocket** - Standalone WebSocket hub. Depends on `gorilla/websocket`.
- **pkg/grpc** - Standalone gRPC utilities. Depends on `google.golang.org/grpc`.
- **pkg/webhook** - Standalone webhook dispatcher. Depends on `net/http` and `crypto/hmac`.
- **pkg/http** - Standalone HTTP client. Depends on `net/http`.

### Abstraction Package (depends on concepts from all others)

- **pkg/transport** - Defines the `Transport` interface and `Factory`. Changes here may affect consumers that use the unified abstraction. Coordinate with agents working on consumer code.

## Coordination Rules

### Rule 1: Interface Changes Require Broadcast

If any agent modifies an exported interface (`Transport`, `StreamServer`, `StreamInterceptor`, `Registry`), all agents working on the module must be notified. Interface changes are breaking changes.

Key interfaces and their locations:
- `transport.Transport` (Send, Receive, Close) in `pkg/transport/transport.go`
- `grpc.StreamServer` (Unary, ServerStream, ClientStream, BidirectionalStream) in `pkg/grpc/grpc.go`
- `grpc.StreamInterceptor` (function type) in `pkg/grpc/grpc.go`

### Rule 2: Config Structs Follow the DefaultConfig Pattern

Every package uses the same configuration pattern:
1. A `Config` struct with exported fields
2. A `DefaultConfig()` function returning sensible defaults
3. Constructor functions accept `*Config` and fall back to `DefaultConfig()` when nil

When adding configuration fields, always:
- Add the field to the `Config` struct with a doc comment
- Set a default in `DefaultConfig()`
- Ensure nil-config still works in the constructor

### Rule 3: Concurrency Safety is Mandatory

All types in this module are designed for concurrent use. Every exported type uses `sync.Mutex` or `sync.RWMutex` for shared state. When modifying any type:
- Identify shared mutable state
- Use `RWMutex` when reads significantly outnumber writes
- Use `Mutex` when write frequency is comparable to reads
- Always defer unlocks
- Never hold two locks simultaneously to avoid deadlocks

### Rule 4: Test Coverage

Every exported function and method must have corresponding tests. The module uses:
- `testify/assert` and `testify/require` for assertions
- Table-driven tests following the `Test<Struct>_<Method>_<Scenario>` naming convention
- Race detection: all tests must pass with `-race`
- Benchmarks for performance-critical paths

### Rule 5: Error Wrapping Convention

All errors must be wrapped with context using `fmt.Errorf("...: %w", err)`. Error messages should describe the operation that failed, not the caller's intent.

## Agent Task Decomposition

When a task spans multiple packages, decompose it as follows:

### Adding a New Transport Type

1. **Agent A** (transport): Add the new `Type` constant to `pkg/transport/transport.go`, register a creator in `NewFactory()`.
2. **Agent B** (implementation): Create the transport-specific package if needed, or implement the creator function.
3. **Agent C** (tests): Write unit tests for the new transport type, integration tests for factory creation.

### Adding Observability (Metrics/Tracing)

1. **Agent A** (sse): Add metrics to `Broker` (client count gauge, events broadcast counter).
2. **Agent B** (websocket): Add metrics to `Hub` (connection count, messages per room).
3. **Agent C** (http): Add metrics to `Client` (request latency histogram, circuit breaker state gauge).
4. **Agent D** (webhook): Add metrics to `Dispatcher` (delivery latency, retry count).
5. **Agent E** (grpc): Add metrics via `StreamInterceptor` (stream duration, message count).

These five agents can work in parallel since the packages are independent.

### Adding Authentication

1. **Agent A** (websocket): Add origin validation enhancement and token-based auth in `ServeWS`.
2. **Agent B** (webhook): Add HMAC verification middleware for incoming webhook endpoints.
3. **Agent C** (grpc): Add auth interceptor via `StreamInterceptor`.

## Conflict Resolution

If two agents modify the same file:
1. The agent making the larger structural change takes priority.
2. The other agent rebases onto the first agent's changes.
3. If both changes are structural, coordinate via comments in the code review.

## Testing After Multi-Agent Changes

After parallel agent work is merged, run the full validation:

```bash
cd Streaming/
go build ./...
go test ./... -count=1 -race
go vet ./...
```

## Communication Protocol

Agents should document their changes in commit messages using Conventional Commits:

```
feat(sse): add event buffering for reconnection replay
fix(websocket): prevent panic on double close
refactor(transport): extract factory registration to separate file
test(webhook): add integration tests for retry backoff
```

Scope must be one of: `sse`, `websocket`, `grpc`, `webhook`, `http`, `transport`, `docs`, `ci`.
