package reqctx

import (
	"context"
	"time"
)

// LoggerContextUpdater is an interface for updating logger context.
// It allows the runtime library to be logger-agnostic while still
// supporting logger context enrichment (e.g., adding ActorUID, RequestID to logs).
//
// Example implementation for zerolog:
//
//	type zerologUpdater struct{}
//
//	func (z *zerologUpdater) UpdateContext(ctx context.Context, update func(LoggerContext) LoggerContext) context.Context {
//	    return zlog.Ctx(ctx).UpdateContext(func(c zlog.Context) zlog.Context {
//	        adapter := &zerologAdapter{c}
//	        updated := update(adapter)
//	        return updated.(*zerologAdapter).ctx
//	    })
//	}
type LoggerContextUpdater interface {
	UpdateContext(ctx context.Context, update func(c LoggerContext) LoggerContext) context.Context
}

// LoggerContext is a minimal interface for updating log context.
// It provides methods to add structured fields to the logger context.
type LoggerContext interface {
	Int64(key string, val int64) LoggerContext
	Str(key string, val string) LoggerContext
	Time(key string, val time.Time) LoggerContext
}

var globalLoggerUpdater LoggerContextUpdater // nil by default

// SetLoggerUpdater sets the global logger updater.
// This should be called once during application initialization.
//
// Example:
//
//	func main() {
//	    reqctx.SetLoggerUpdater(&zerologUpdater{})
//	    // ... rest of initialization
//	}
func SetLoggerUpdater(updater LoggerContextUpdater) {
	globalLoggerUpdater = updater
}
