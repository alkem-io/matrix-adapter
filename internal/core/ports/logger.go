// Package ports defines the interfaces (ports) for the hexagonal architecture.
package ports

// Logger defines the interface for logging.
type Logger interface {
	// Debug logs a debug-level message with optional structured fields.
	Debug(msg string, fields ...interface{})
	// Info logs an info-level message with optional structured fields.
	Info(msg string, fields ...interface{})
	// Warn logs a warning-level message with optional structured fields.
	Warn(msg string, fields ...interface{})
	// Error logs an error-level message with optional structured fields.
	Error(msg string, fields ...interface{})
	// With returns a new Logger with the given fields added to its context.
	With(fields ...interface{}) Logger
}
