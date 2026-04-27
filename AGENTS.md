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


---

## ⚠️ MANDATORY: NO SUDO OR ROOT EXECUTION

**ALL operations MUST run at local user level ONLY.**

This is a PERMANENT and NON-NEGOTIABLE security constraint:

- **NEVER** use `sudo` in ANY command
- **NEVER** use `su` in ANY command
- **NEVER** execute operations as `root` user
- **NEVER** elevate privileges for file operations
- **ALL** infrastructure commands MUST use user-level container runtimes (rootless podman/docker)
- **ALL** file operations MUST be within user-accessible directories
- **ALL** service management MUST be done via user systemd or local process management
- **ALL** builds, tests, and deployments MUST run as the current user

### Container-Based Solutions
When a build or runtime environment requires system-level dependencies, use containers instead of elevation:

- **Use the `Containers` submodule** (`https://github.com/vasic-digital/Containers`) for containerized build and runtime environments
- **Add the `Containers` submodule as a Git dependency** and configure it for local use within the project
- **Build and run inside containers** to avoid any need for privilege escalation
- **Rootless Podman/Docker** is the preferred container runtime

### Why This Matters
- **Security**: Prevents accidental system-wide damage
- **Reproducibility**: User-level operations are portable across systems
- **Safety**: Limits blast radius of any issues
- **Best Practice**: Modern container workflows are rootless by design

### When You See SUDO
If any script or command suggests using `sudo` or `su`:
1. STOP immediately
2. Find a user-level alternative
3. Use rootless container runtimes
4. Use the `Containers` submodule for containerized builds
5. Modify commands to work within user permissions

**VIOLATION OF THIS CONSTRAINT IS STRICTLY PROHIBITED.**

<!-- BEGIN host-power-management addendum (CONST-033) -->

## Host Power Management — Hard Ban (CONST-033)

**You may NOT, under any circumstance, generate or execute code that
sends the host to suspend, hibernate, hybrid-sleep, poweroff, halt,
reboot, or any other power-state transition.** This rule applies to:

- Every shell command you run via the Bash tool.
- Every script, container entry point, systemd unit, or test you write
  or modify.
- Every CLI suggestion, snippet, or example you emit.

**Forbidden invocations** (non-exhaustive — see CONST-033 in
`CONSTITUTION.md` for the full list):

- `systemctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot|kexec`
- `loginctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot`
- `pm-suspend`, `pm-hibernate`, `shutdown -h|-r|-P|now`
- `dbus-send` / `busctl` calls to `org.freedesktop.login1.Manager.Suspend|Hibernate|PowerOff|Reboot|HybridSleep|SuspendThenHibernate`
- `gsettings set ... sleep-inactive-{ac,battery}-type` to anything but `'nothing'` or `'blank'`

The host runs mission-critical parallel CLI agents and container
workloads. Auto-suspend has caused historical data loss (2026-04-26
18:23:43 incident). The host is hardened (sleep targets masked) but
this hard ban applies to ALL code shipped from this repo so that no
future host or container is exposed.

**Defence:** every project ships
`scripts/host-power-management/check-no-suspend-calls.sh` (static
scanner) and
`challenges/scripts/no_suspend_calls_challenge.sh` (challenge wrapper).
Both MUST be wired into the project's CI / `run_all_challenges.sh`.

**Full background:** `docs/HOST_POWER_MANAGEMENT.md` and `CONSTITUTION.md` (CONST-033).

<!-- END host-power-management addendum (CONST-033) -->



<!-- CONST-035 anti-bluff addendum (cascaded) -->

## CONST-035 — Anti-Bluff Tests & Challenges (mandatory; inherits from root)

Tests and Challenges in this submodule MUST verify the product, not
the LLM's mental model of the product. A test that passes when the
feature is broken is worse than a missing test — it gives false
confidence and lets defects ship to users. Functional probes at the
protocol layer are mandatory:

- TCP-open is the FLOOR, not the ceiling. Postgres → execute
  `SELECT 1`. Redis → `PING` returns `PONG`. ChromaDB → `GET
  /api/v1/heartbeat` returns 200. MCP server → TCP connect + valid
  JSON-RPC handshake. HTTP gateway → real request, real response,
  non-empty body.
- Container `Up` is NOT application healthy. A `docker/podman ps`
  `Up` status only means PID 1 is running; the application may be
  crash-looping internally.
- No mocks/fakes outside unit tests (already CONST-030; CONST-035
  raises the cost of a mock-driven false pass to the same severity
  as a regression).
- Re-verify after every change. Don't assume a previously-passing
  test still verifies the same scope after a refactor.
- Verification of CONST-035 itself: deliberately break the feature
  (e.g. `kill <service>`, swap a password). The test MUST fail. If
  it still passes, the test is non-conformant and MUST be tightened.

## CONST-033 clarification — distinguishing host events from sluggishness

Heavy container builds (BuildKit pulling many GB of layers, parallel
podman/docker compose-up across many services) can make the host
**appear** unresponsive — high load average, slow SSH, watchers
timing out. **This is NOT a CONST-033 violation.** Suspend / hibernate
/ logout are categorically different events. Distinguish via:

- `uptime` — recent boot? if so, the host actually rebooted.
- `loginctl list-sessions` — session(s) still active? if yes, no logout.
- `journalctl ... | grep -i 'will suspend\|hibernate'` — zero broadcasts
  since the CONST-033 fix means no suspend ever happened.
- `dmesg | grep -i 'killed process\|out of memory'` — OOM kills are
  also NOT host-power events; they're memory-pressure-induced and
  require their own separate fix (lower per-container memory limits,
  reduce parallelism).

A sluggish host under build pressure recovers when the build finishes;
a suspended host requires explicit unsuspend (and CONST-033 should
make that impossible by hardening `IdleAction=ignore` +
`HandleSuspendKey=ignore` + masked `sleep.target`,
`suspend.target`, `hibernate.target`, `hybrid-sleep.target`).

If you observe what looks like a suspend during heavy builds, the
correct first action is **not** "edit CONST-033" but `bash
challenges/scripts/host_no_auto_suspend_challenge.sh` to confirm the
hardening is intact. If hardening is intact AND no suspend
broadcast appears in journal, the perceived event was build-pressure
sluggishness, not a power transition.
