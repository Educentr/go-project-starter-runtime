package logger

import (
	"context"
	"time"

	"github.com/Educentr/go-project-starter-runtime/pkg/reqctx"
	"github.com/sirupsen/logrus"
)

type logrusEntryCtxKey struct{}

// AppLogrusLogger stores the logrus entry creator function.
type AppLogrusLogger struct {
	logrusCreator func(ocPrefix, ocPath string) *logrus.Entry
}

// InitAppLogrus creates a new AppLogrusLogger with the given creator function.
func InitAppLogrus(f func(ocPrefix, ocPath string) *logrus.Entry) AppLogrusLogger {
	return AppLogrusLogger{
		logrusCreator: f,
	}
}

// FromContextLoggerLogrus retrieves the AppLogrusLogger from context.
// Panics if the logger is not found in context.
func FromContextLoggerLogrus(ctx context.Context) AppLogrusLogger {
	logI := ctx.Value(loggerContextKey)
	if log, ok := logI.(AppLogrusLogger); ok {
		return log
	}

	panic("AppLogrusLogger not found in context")
}

// LogrusFromContext retrieves the *logrus.Entry from context.
// Returns a default entry from the standard logger if not found.
func LogrusFromContext(ctx context.Context) *logrus.Entry {
	if entry, ok := ctx.Value(logrusEntryCtxKey{}).(*logrus.Entry); ok {
		return entry
	}

	return logrus.NewEntry(logrus.StandardLogger())
}

// LogrusToContext stores a *logrus.Entry in context.
func LogrusToContext(ctx context.Context, entry *logrus.Entry) context.Context {
	return context.WithValue(ctx, logrusEntryCtxKey{}, entry)
}

// WrapLogger implements AppLogger interface.
// Creates a new logrus entry in ctx using the stored creator function.
func (l AppLogrusLogger) WrapLogger(ctx context.Context, ocPrefix, ocPath string) context.Context {
	entry := l.logrusCreator(ocPrefix, ocPath)

	return LogrusToContext(ctx, entry)
}

// CopyFields implements AppLogger interface.
// Copies all logrus fields from source context's entry to destination context's entry.
func (l AppLogrusLogger) CopyFields(source, destination context.Context) context.Context {
	sourceEntry := LogrusFromContext(source)
	destEntry := LogrusFromContext(destination)

	for k, v := range sourceEntry.Data {
		destEntry = destEntry.WithField(k, v)
	}

	return LogrusToContext(destination, destEntry)
}

// LogrusUpdater implements reqctx.LoggerContextUpdater for logrus.
type LogrusUpdater struct{}

// NewLogrusUpdater creates a new LogrusUpdater.
func NewLogrusUpdater() *LogrusUpdater {
	return &LogrusUpdater{}
}

// UpdateContext implements reqctx.LoggerContextUpdater.
func (l *LogrusUpdater) UpdateContext(ctx context.Context, update func(c reqctx.LoggerContext) reqctx.LoggerContext) context.Context {
	entry := LogrusFromContext(ctx)
	adapter := &logrusAdapter{entry: entry}
	updated := update(adapter)

	return LogrusToContext(ctx, updated.(*logrusAdapter).entry)
}

// logrusAdapter adapts *logrus.Entry to reqctx.LoggerContext.
type logrusAdapter struct {
	entry *logrus.Entry
}

func (a *logrusAdapter) Int64(key string, val int64) reqctx.LoggerContext {
	a.entry = a.entry.WithField(key, val)

	return a
}

func (a *logrusAdapter) Str(key string, val string) reqctx.LoggerContext {
	a.entry = a.entry.WithField(key, val)

	return a
}

func (a *logrusAdapter) Time(key string, val time.Time) reqctx.LoggerContext {
	a.entry = a.entry.WithField(key, val)

	return a
}
