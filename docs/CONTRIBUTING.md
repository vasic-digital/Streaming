# Contributing

## Prerequisites

- Go 1.24 or later
- `gofmt`, `goimports`, `golangci-lint`, `gosec` installed
- Familiarity with the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

## Getting Started

```bash
git clone <repository-url>
cd Streaming
go mod download
go build ./...
go test ./... -count=1 -race
```

## Development Workflow

### 1. Branch Naming

Create a branch from `main` using the following prefixes:

| Prefix | Use Case |
|--------|----------|
| `feat/` | New feature |
| `fix/` | Bug fix |
| `refactor/` | Code restructuring without behavior change |
| `test/` | Adding or updating tests |
| `docs/` | Documentation changes |
| `chore/` | Build, CI, dependency updates |

Example: `feat/sse-event-replay`

### 2. Making Changes

- Follow the existing patterns in the codebase. Every package uses the `Config` / `DefaultConfig()` / nil-safe constructor pattern.
- All exported types must be safe for concurrent use.
- Keep packages independent -- do not introduce cross-dependencies between the six `pkg/` packages.
- Line length should not exceed 100 characters.

### 3. Code Style

**Imports**: Group in three blocks separated by blank lines -- stdlib, third-party, internal:

```go
import (
    "context"
    "fmt"
    "sync"

    "github.com/gorilla/websocket"

    "digital.vasic.streaming/pkg/transport"
)
```

**Naming**:
- `camelCase` for unexported identifiers
- `PascalCase` for exported identifiers
- `UPPER_SNAKE_CASE` for constants
- Acronyms in all caps: `HTTP`, `URL`, `ID`, `SSE`, `TLS`
- Receivers: 1-2 letter abbreviations (`b` for Broker, `h` for Hub, `d` for Dispatcher)

**Errors**:
- Always check errors
- Wrap with context: `fmt.Errorf("failed to send event: %w", err)`
- Use `defer` for cleanup

### 4. Testing

Every exported function and method must have tests. Follow these conventions:

**Test naming**: `Test<Struct>_<Method>_<Scenario>`

```go
func TestBroker_Broadcast_SendsToAllClients(t *testing.T) { ... }
func TestBroker_SendTo_ClientNotFound(t *testing.T) { ... }
func TestBroker_AddClient_MaxClientsReached(t *testing.T) { ... }
```

**Table-driven tests**:

```go
func TestEvent_Format(t *testing.T) {
    tests := []struct {
        name     string
        event    sse.Event
        expected string
    }{
        {
            name:     "with all fields",
            event:    sse.Event{ID: "1", Type: "msg", Data: []byte("hi"), Retry: 5000},
            expected: "id: 1\nevent: msg\nretry: 5000\ndata: hi\n\n",
        },
        {
            name:     "data only",
            event:    sse.Event{Data: []byte("hi")},
            expected: "data: hi\n\n",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.expected, string(tt.event.Format()))
        })
    }
}
```

**Assertions**: Use `testify/assert` for non-fatal checks and `testify/require` for fatal preconditions.

**Run tests**:

```bash
go test ./... -count=1 -race       # All tests with race detection
go test ./... -short                # Unit tests only
go test -bench=. ./...              # Benchmarks
go test -tags=integration ./...    # Integration tests
go test -coverprofile=coverage.out ./...  # Coverage
go tool cover -html=coverage.out         # View coverage report
```

### 5. Pre-Commit Checks

Run these before every commit:

```bash
go fmt ./...
go vet ./...
golangci-lint run ./...
go test ./... -count=1 -race
```

### 6. Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

**Types**: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`

**Scopes**: `sse`, `websocket`, `grpc`, `webhook`, `http`, `transport`, `docs`, `ci`

**Examples**:

```
feat(sse): add event replay buffer for reconnecting clients
fix(websocket): prevent data race in room broadcast
refactor(http): extract backoff calculation to shared utility
test(webhook): add HMAC verification edge cases
docs(transport): document custom transport registration
```

## Adding a New Package

If you need to add a new streaming package:

1. Create `pkg/<name>/<name>.go` with the package documentation comment.
2. Define a `Config` struct and `DefaultConfig()` function.
3. Implement the main type with a nil-safe constructor (`NewXxx(config *Config)`).
4. Ensure all exported types are concurrency-safe.
5. Create `pkg/<name>/<name>_test.go` with comprehensive tests.
6. Update `docs/API_REFERENCE.md` with the new package's API.
7. Update `docs/ARCHITECTURE.md` with design decisions.
8. Update `CLAUDE.md` with the new package in the table.

## Adding a New Transport Type

1. Define a new `Type` constant in `pkg/transport/transport.go`.
2. Implement a creator function: `func(config *Config) (Transport, error)`.
3. Register it in `NewFactory()` or document it for user registration.
4. Add tests for the new transport type.

## Pull Request Checklist

Before submitting a PR, ensure:

- [ ] All tests pass: `go test ./... -count=1 -race`
- [ ] Code is formatted: `go fmt ./...`
- [ ] No vet warnings: `go vet ./...`
- [ ] Linter passes: `golangci-lint run ./...`
- [ ] New exports have doc comments
- [ ] New types are concurrency-safe
- [ ] Tests follow `Test<Struct>_<Method>_<Scenario>` naming
- [ ] Commit messages follow Conventional Commits
- [ ] Documentation is updated if API changed

## Reporting Issues

When reporting a bug, include:

1. Go version (`go version`)
2. Module version
3. Minimal reproduction code
4. Expected vs actual behavior
5. Stack trace if applicable
