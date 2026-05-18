// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Milos Vasic

package i18n_test

import (
	"context"
	"testing"

	"digital.vasic.streaming/pkg/i18n"
)

// TestNoopTranslator_T_ReturnsMsgIDVerbatim asserts that the
// stripped-down fallback Translator emits the message ID unchanged.
// Per CONST-035 / Article XI §11.9 this verbatim-fallback is itself
// positive runtime evidence — operators see exactly which key was
// resolved without a bundle.
func TestNoopTranslator_T_ReturnsMsgIDVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}
	got := tr.T(context.Background(), "streaming_sse_unsupported_writer", map[string]any{
		"limit": 100,
	})
	const want = "streaming_sse_unsupported_writer"
	if got != want {
		t.Fatalf("NoopTranslator.T mismatch:\n got = %q\nwant = %q", got, want)
	}
}

// TestNoopTranslator_TPlural_ReturnsMsgIDVerbatim mirrors the T
// assertion for plural-form lookups.
func TestNoopTranslator_TPlural_ReturnsMsgIDVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}
	got := tr.TPlural(context.Background(), "streaming_sse_max_clients_reached", 3, nil)
	const want = "streaming_sse_max_clients_reached"
	if got != want {
		t.Fatalf("NoopTranslator.TPlural mismatch:\n got = %q\nwant = %q", got, want)
	}
}

// TestNoopTranslator_T_NilArgs_ReturnsMsgIDVerbatim ensures the noop
// implementation tolerates nil arg maps without panic — important for
// call-sites that have no template substitutions (e.g. the
// transport package's "config is required" error).
func TestNoopTranslator_T_NilArgs_ReturnsMsgIDVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}
	got := tr.T(context.Background(), "streaming_transport_config_required", nil)
	const want = "streaming_transport_config_required"
	if got != want {
		t.Fatalf("NoopTranslator.T(nil args) mismatch:\n got = %q\nwant = %q", got, want)
	}
}
