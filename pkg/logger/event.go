package logger

import "context"

// EventLogger provides structured logging for emitting events (errors, warnings, info).
// Unlike AppLogger (which handles context wrapping and field propagation),
// EventLogger is used to output log messages with structured fields.
type EventLogger interface {
	Error(ctx context.Context, msg string, err error, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
}

// Field is a key-value pair for structured logging.
type Field struct {
	Key   string
	Value any
}

// Str creates a string field.
func Str(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Int creates an int field.
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Int64 creates an int64 field.
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

// Any creates a field with an arbitrary value.
func Any(key string, value any) Field {
	return Field{Key: key, Value: value}
}

var globalEventLogger EventLogger

// SetEventLogger sets the global EventLogger instance.
// Should be called during application initialization.
func SetEventLogger(l EventLogger) {
	globalEventLogger = l
}

// GetEventLogger returns the global EventLogger.
// Returns a no-op logger if none was set.
func GetEventLogger() EventLogger {
	if globalEventLogger == nil {
		return nopEventLogger{}
	}

	return globalEventLogger
}

// nopEventLogger is a no-op implementation used when no logger is configured.
type nopEventLogger struct{}

func (nopEventLogger) Error(context.Context, string, error, ...Field) {}
func (nopEventLogger) Warn(context.Context, string, ...Field)        {}
func (nopEventLogger) Info(context.Context, string, ...Field)        {}
