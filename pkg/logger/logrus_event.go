package logger

import (
	"context"

	"github.com/sirupsen/logrus"
)

// LogrusEventLogger implements EventLogger using logrus.
type LogrusEventLogger struct{}

// NewLogrusEventLogger creates a new LogrusEventLogger.
func NewLogrusEventLogger() *LogrusEventLogger {
	return &LogrusEventLogger{}
}

func (l *LogrusEventLogger) Error(ctx context.Context, msg string, err error, fields ...Field) {
	entry := LogrusFromContext(ctx)
	if err != nil {
		entry = entry.WithError(err)
	}

	applyFieldsLogrus(entry, fields).Error(msg)
}

func (l *LogrusEventLogger) Warn(ctx context.Context, msg string, fields ...Field) {
	entry := LogrusFromContext(ctx)
	applyFieldsLogrus(entry, fields).Warn(msg)
}

func (l *LogrusEventLogger) Info(ctx context.Context, msg string, fields ...Field) {
	entry := LogrusFromContext(ctx)
	applyFieldsLogrus(entry, fields).Info(msg)
}

func applyFieldsLogrus(entry *logrus.Entry, fields []Field) *logrus.Entry {
	for _, f := range fields {
		entry = entry.WithField(f.Key, f.Value)
	}

	return entry
}
