// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Milos Vasic

package transport_test

import (
	"strings"
	"testing"

	"digital.vasic.streaming/pkg/transport"
)

// TestTransport_FactoryCreate_NilConfig_EmitsMsgIDInError asserts
// that Factory.Create(nil) returns an error containing the namespaced
// message ID verbatim under the default NoopTranslator. Per CONST-035
// / Article XI §11.9 the message-ID-in-error is itself positive
// runtime evidence.
func TestTransport_FactoryCreate_NilConfig_EmitsMsgIDInError(t *testing.T) {
	// Ensure no leftover Translator from a sibling test affects the
	// default behaviour. Setting nil resets to NoopTranslator{}.
	transport.SetTranslator(nil)

	f := transport.NewFactory()
	_, err := f.Create(nil)
	if err == nil {
		t.Fatal("Factory.Create(nil): want error, got nil")
	}
	const want = "streaming_transport_config_required"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Factory.Create error: got %q, want substring %q (noop fallback must emit msgID verbatim)",
			err.Error(), want)
	}
}
