package realtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNoopLogger_Info_Coverage exercises the noopLogger.Info method body
// to achieve statement coverage on line 248.
func TestNoopLogger_Info_Coverage(t *testing.T) {
	n := &noopLogger{}
	assert.NotPanics(t, func() {
		n.Info("coverage message")
	})
}

// TestNoopLogger_Warn_Coverage exercises the noopLogger.Warn method body
// to achieve statement coverage on line 249.
func TestNoopLogger_Warn_Coverage(t *testing.T) {
	n := &noopLogger{}
	assert.NotPanics(t, func() {
		n.Warn("coverage message")
	})
}

// TestNoopLogger_Error_Coverage exercises the noopLogger.Error method body
// to achieve statement coverage on line 250.
func TestNoopLogger_Error_Coverage(t *testing.T) {
	n := &noopLogger{}
	assert.NotPanics(t, func() {
		n.Error("coverage message")
	})
}

// TestNoopLogger_Debug_Coverage exercises the noopLogger.Debug method body
// to achieve statement coverage on line 251.
func TestNoopLogger_Debug_Coverage(t *testing.T) {
	n := &noopLogger{}
	assert.NotPanics(t, func() {
		n.Debug("coverage message")
	})
}

// TestNoopLogger_WithArgs_Coverage verifies the noopLogger methods accept
// variadic key-value arguments without panicking.
func TestNoopLogger_WithArgs_Coverage(t *testing.T) {
	// bluff-scan: no-assert-ok (null-implementation smoke — no-op type must accept all interface calls without panic)
	n := &noopLogger{}

	n.Info("msg", "key1", "value1", "key2", 42)
	n.Warn("msg", "key1", "value1")
	n.Error("msg", "error", "something went wrong")
	n.Debug("msg", "detail", true, "count", 7)
}

// TestNoopLogger_ImplementsLogger_Coverage verifies at compile time that
// noopLogger satisfies the Logger interface.
func TestNoopLogger_ImplementsLogger_Coverage(t *testing.T) {
	var _ Logger = &noopLogger{}
}
