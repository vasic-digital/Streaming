#!/usr/bin/env bash
# streaming_functionality_challenge.sh - Validates Streaming module core functionality
# Checks SSE, WebSocket, gRPC, HTTP client, webhook, and transport types
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="Streaming"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== ${MODULE_NAME} Functionality Challenge ==="
echo ""

# --- Section 1: Required packages ---
echo "Section 1: Required packages (6)"

for pkg in sse websocket grpc http webhook transport; do
    echo "Test: Package pkg/${pkg} exists"
    if [ -d "${MODULE_DIR}/pkg/${pkg}" ]; then
        pass "Package pkg/${pkg} exists"
    else
        fail "Package pkg/${pkg} missing"
    fi
done

# --- Section 2: SSE ---
echo ""
echo "Section 2: SSE (Server-Sent Events)"

echo "Test: SSE Event struct exists"
if grep -q "type Event struct" "${MODULE_DIR}/pkg/sse/"*.go 2>/dev/null; then
    pass "SSE Event struct exists"
else
    fail "SSE Event struct missing"
fi

echo "Test: SSE Broker struct exists"
if grep -q "type Broker struct" "${MODULE_DIR}/pkg/sse/"*.go 2>/dev/null; then
    pass "SSE Broker struct exists"
else
    fail "SSE Broker struct missing"
fi

echo "Test: SSE Client struct exists"
if grep -q "type Client struct" "${MODULE_DIR}/pkg/sse/"*.go 2>/dev/null; then
    pass "SSE Client struct exists"
else
    fail "SSE Client struct missing"
fi

# --- Section 3: WebSocket ---
echo ""
echo "Section 3: WebSocket"

echo "Test: WebSocket Client struct exists"
if grep -q "type Client struct" "${MODULE_DIR}/pkg/websocket/"*.go 2>/dev/null; then
    pass "WebSocket Client struct exists"
else
    fail "WebSocket Client struct missing"
fi

echo "Test: WebSocket Hub struct exists"
if grep -q "type Hub struct" "${MODULE_DIR}/pkg/websocket/"*.go 2>/dev/null; then
    pass "WebSocket Hub struct exists"
else
    fail "WebSocket Hub struct missing"
fi

echo "Test: WebSocket Room struct exists"
if grep -q "type Room struct" "${MODULE_DIR}/pkg/websocket/"*.go 2>/dev/null; then
    pass "WebSocket Room struct exists"
else
    fail "WebSocket Room struct missing"
fi

echo "Test: WebSocket Message struct exists"
if grep -q "type Message struct" "${MODULE_DIR}/pkg/websocket/"*.go 2>/dev/null; then
    pass "WebSocket Message struct exists"
else
    fail "WebSocket Message struct missing"
fi

# --- Section 4: gRPC ---
echo ""
echo "Section 4: gRPC"

echo "Test: gRPC StreamServer interface exists"
if grep -q "type StreamServer interface" "${MODULE_DIR}/pkg/grpc/"*.go 2>/dev/null; then
    pass "gRPC StreamServer interface exists"
else
    fail "gRPC StreamServer interface missing"
fi

echo "Test: gRPC HealthServer struct exists"
if grep -q "type HealthServer struct" "${MODULE_DIR}/pkg/grpc/"*.go 2>/dev/null; then
    pass "gRPC HealthServer struct exists"
else
    fail "gRPC HealthServer struct missing"
fi

# --- Section 5: Transport abstraction ---
echo ""
echo "Section 5: Transport abstraction"

echo "Test: Transport interface exists"
if grep -q "type Transport interface" "${MODULE_DIR}/pkg/transport/"*.go 2>/dev/null; then
    pass "Transport interface exists"
else
    fail "Transport interface missing"
fi

echo "Test: Factory struct exists"
if grep -q "type Factory struct" "${MODULE_DIR}/pkg/transport/"*.go 2>/dev/null; then
    pass "Factory struct exists"
else
    fail "Factory struct missing"
fi

# --- Section 6: Webhook ---
echo ""
echo "Section 6: Webhook"

echo "Test: Webhook struct exists"
if grep -q "type Webhook struct" "${MODULE_DIR}/pkg/webhook/"*.go 2>/dev/null; then
    pass "Webhook struct exists"
else
    fail "Webhook struct missing"
fi

echo "Test: Dispatcher struct exists"
if grep -q "type Dispatcher struct" "${MODULE_DIR}/pkg/webhook/"*.go 2>/dev/null; then
    pass "Dispatcher struct exists"
else
    fail "Dispatcher struct missing"
fi

echo "Test: Registry struct exists"
if grep -q "type Registry struct" "${MODULE_DIR}/pkg/webhook/"*.go 2>/dev/null; then
    pass "Registry struct exists"
else
    fail "Registry struct missing"
fi

# --- Section 7: HTTP client ---
echo ""
echo "Section 7: HTTP client"

echo "Test: HTTP Client struct exists"
if grep -q "type Client struct" "${MODULE_DIR}/pkg/http/"*.go 2>/dev/null; then
    pass "HTTP Client struct exists"
else
    fail "HTTP Client struct missing"
fi

echo "Test: HTTP Response struct exists"
if grep -q "type Response struct" "${MODULE_DIR}/pkg/http/"*.go 2>/dev/null; then
    pass "HTTP Response struct exists"
else
    fail "HTTP Response struct missing"
fi

# --- Section 8: Source structure completeness ---
echo ""
echo "Section 8: Source structure"

echo "Test: Each package has non-test Go source files"
all_have_source=true
for pkg in sse websocket grpc http webhook transport; do
    non_test=$(find "${MODULE_DIR}/pkg/${pkg}" -name "*.go" ! -name "*_test.go" -type f 2>/dev/null | wc -l)
    if [ "$non_test" -eq 0 ]; then
        fail "Package pkg/${pkg} has no non-test Go files"
        all_have_source=false
    fi
done
if [ "$all_have_source" = true ]; then
    pass "All packages have non-test Go source files"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
