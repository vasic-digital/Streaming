// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Milos Vasic

package sse_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"digital.vasic.streaming/pkg/sse"
)

// noFlusherResponseWriter wraps a *httptest.ResponseRecorder but does
// NOT implement http.Flusher — required to drive the SSE broker into
// its "streaming unsupported by response writer" branch.
type noFlusherResponseWriter struct {
	headers http.Header
	body    *strings.Builder
	code    int
}

func newNoFlusherResponseWriter() *noFlusherResponseWriter {
	return &noFlusherResponseWriter{
		headers: http.Header{},
		body:    &strings.Builder{},
	}
}

func (w *noFlusherResponseWriter) Header() http.Header { return w.headers }
func (w *noFlusherResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}
func (w *noFlusherResponseWriter) WriteHeader(code int) { w.code = code }

// TestSSE_AddClient_NonFlusherWriter_EmitsMsgIDInError asserts that
// when the response writer does not implement http.Flusher the
// returned error contains the namespaced message ID verbatim under
// the default NoopTranslator. Per CONST-035 / Article XI §11.9 the
// message-ID-in-error is itself positive runtime evidence.
func TestSSE_AddClient_NonFlusherWriter_EmitsMsgIDInError(t *testing.T) {
	broker := sse.NewBroker(nil)
	defer broker.Close()

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	_, err := broker.AddClient(newNoFlusherResponseWriter(), req)
	if err == nil {
		t.Fatal("AddClient on non-flusher writer: want error, got nil")
	}
	const want = "streaming_sse_unsupported_writer"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("AddClient error: got %q, want substring %q (noop fallback must emit msgID verbatim)",
			err.Error(), want)
	}
}

// TestSSE_AddClient_MaxClientsReached_EmitsMsgIDInError exercises the
// second migrated literal: when MaxClients is configured to 0 (the
// "no clients allowed" smoke configuration) the broker MUST refuse
// the first connection with the namespaced message ID verbatim.
func TestSSE_AddClient_MaxClientsReached_EmitsMsgIDInError(t *testing.T) {
	// MaxClients=0 is a special value meaning unlimited per the
	// existing code, so we use MaxClients=1 + count=1 simulation
	// via a pre-existing client. Simpler: use MaxClients=1 then add
	// one client successfully (with flusher), then add a second to
	// trigger the limit branch.
	broker := sse.NewBroker(&sse.Config{
		BufferSize:        1,
		MaxClients:        1,
		HeartbeatInterval: 30 * time.Second,
	})
	defer broker.Close()

	// First client with a flushable recorder succeeds.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/events", nil)
	if _, err := broker.AddClient(rec1, req1); err != nil {
		t.Fatalf("first AddClient: unexpected error %v", err)
	}

	// Second client trips the MaxClients limit.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/events", nil)
	_, err := broker.AddClient(rec2, req2)
	if err == nil {
		t.Fatal("second AddClient over MaxClients=1: want error, got nil")
	}
	const want = "streaming_sse_max_clients_reached"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("AddClient max-clients error: got %q, want substring %q",
			err.Error(), want)
	}
}
