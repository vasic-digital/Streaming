// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Milos Vasic

package websocket_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	ws "github.com/gorilla/websocket"

	"digital.vasic.streaming/pkg/websocket"
)

// TestWebSocket_ClientSend_AfterClose_EmitsMsgIDInError asserts that
// calling Send on a closed Client returns the namespaced message ID
// verbatim under the default NoopTranslator. Per CONST-035 / Article
// XI §11.9 the message-ID-in-error is itself positive runtime
// evidence.
func TestWebSocket_ClientSend_AfterClose_EmitsMsgIDInError(t *testing.T) {
	hub := websocket.NewHub(nil)
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := hub.ServeWS(w, r)
		if err != nil {
			t.Logf("ServeWS error (expected for handshake test): %v", err)
			return
		}
		// Immediately close the client to force Send to hit the
		// closed-client branch on the next call.
		_ = client.Close()
		sendErr := client.Send([]byte("hello"))
		if sendErr == nil {
			t.Error("Send on closed client: want error, got nil")
			return
		}
		const want = "streaming_websocket_client_closed"
		if !strings.Contains(sendErr.Error(), want) {
			t.Errorf("Send error: got %q, want substring %q",
				sendErr.Error(), want)
		}
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	conn, _, err := ws.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	// Drive the handler to completion by reading until the connection
	// is closed by the server-side defer.
	_, _, _ = conn.ReadMessage()
}
