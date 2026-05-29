package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

func TestNewZapLogger_Debug(t *testing.T) {
	logger, err := NewZapLogger("debug")
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestNewZapLogger_Info(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestNewZapLogger_Warn(t *testing.T) {
	logger, err := NewZapLogger("warn")
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestNewZapLogger_Error(t *testing.T) {
	logger, err := NewZapLogger("error")
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestNewZapLogger_InvalidLevel_FallsBack(t *testing.T) {
	// An unrecognised level string won't fail Build; zapcore.ParseLevel returns an error
	// but the code continues with the default level from the config preset.
	logger, err := NewZapLogger("bogus")
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestZapLogger_ImplementsPortsLogger(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)

	// Compile-time check is implicit (NewZapLogger returns ports.Logger),
	// but verify at runtime too.
	// Verify implements ports.Logger interface
	var iface ports.Logger = logger //nolint:staticcheck // explicit interface check
	_ = iface
}

func TestZapLogger_DebugDoesNotPanic(t *testing.T) {
	logger, err := NewZapLogger("debug")
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		logger.Debug("test debug message")
	})
	assert.NotPanics(t, func() {
		logger.Debug("test debug with fields", "key", "value")
	})
}

func TestZapLogger_InfoDoesNotPanic(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		logger.Info("test info message")
	})
	assert.NotPanics(t, func() {
		logger.Info("test info with fields", "key", "value", "count", 42)
	})
}

func TestZapLogger_WarnDoesNotPanic(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		logger.Warn("test warn message")
	})
}

func TestZapLogger_ErrorDoesNotPanic(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		logger.Error("test error message")
	})
	assert.NotPanics(t, func() {
		logger.Error("test error with fields", "err", "something broke")
	})
}

func TestZapLogger_With(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)

	child := logger.With("request_id", "abc-123")
	assert.NotNil(t, child)

	// Child should be a distinct logger (not the same pointer).
	assert.NotSame(t, logger, child)
}

func TestZapLogger_WithDoesNotPanic(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		child := logger.With("component", "test")
		child.Info("message from child logger")
	})
}

func TestZapLogger_Underlying(t *testing.T) {
	logger, err := NewZapLogger("info")
	require.NoError(t, err)

	zl, ok := logger.(*ZapLogger)
	require.True(t, ok, "expected *ZapLogger")

	underlying := zl.Underlying()
	assert.NotNil(t, underlying, "Underlying() should return a non-nil *zap.Logger")
}
