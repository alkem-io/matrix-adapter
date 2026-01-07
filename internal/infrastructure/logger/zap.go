// Package logger provides logging implementations.
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
)

// ZapLogger is a Logger implementation using Uber's zap library.
type ZapLogger struct {
	logger *zap.SugaredLogger
}

// NewZapLogger creates a new ZapLogger with the specified log level.
func NewZapLogger(level string) (ports.Logger, error) {
	var config zap.Config
	if level == "debug" {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	// Parse level
	l, err := zapcore.ParseLevel(level)
	if err == nil {
		config.Level = zap.NewAtomicLevelAt(l)
	}

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &ZapLogger{
		logger: logger.Sugar(),
	}, nil
}

// Debug logs a message at Debug level.
func (l *ZapLogger) Debug(msg string, fields ...interface{}) {
	l.logger.Debugw(msg, fields...)
}

// Info logs a message at Info level.
func (l *ZapLogger) Info(msg string, fields ...interface{}) {
	l.logger.Infow(msg, fields...)
}

// Warn logs a message at Warn level.
func (l *ZapLogger) Warn(msg string, fields ...interface{}) {
	l.logger.Warnw(msg, fields...)
}

// Error logs a message at Error level.
func (l *ZapLogger) Error(msg string, fields ...interface{}) {
	l.logger.Errorw(msg, fields...)
}

// With returns a new Logger instance with the specified fields added to the context.
func (l *ZapLogger) With(fields ...interface{}) ports.Logger {
	return &ZapLogger{
		logger: l.logger.With(fields...),
	}
}

// Underlying returns the underlying *zap.Logger for direct access when needed.
func (l *ZapLogger) Underlying() *zap.Logger {
	return l.logger.Desugar()
}
