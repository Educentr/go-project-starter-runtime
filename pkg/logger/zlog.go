package logger

import (
	"context"
	"time"

	"github.com/Educentr/go-project-starter-runtime/pkg/reqctx"
	zlog "github.com/rs/zerolog"
)

// AppZlogLogger структура хранящая функцию создания инстанса логгера
type AppZlogLogger struct {
	zlogCreator func(ocPrefix, ocPath string) *zlog.Logger
}

// InitAppZlog конструктор для логгера zlog
func InitAppZlog(f func(ocPrefix, ocPath string) *zlog.Logger) AppZlogLogger {
	return AppZlogLogger{
		zlogCreator: f,
	}
}

// FromContextLoggerZlog достаёт логгер из контекста
// Паникует в случае отсутствия логгера в контексте
func FromContextLoggerZlog(ctx context.Context) AppZlogLogger {
	logI := ctx.Value(loggerContextKey)
	if log, ok := logI.(AppZlogLogger); ok {
		return log
	}

	panic("AppZlogLogger not found in context")
}

// ReWrapZlog позволяет переложить логгер из одного контекста в другой
// а так же создать инстанс zlog-а в контексте получателе
func ReWrapZlog(source context.Context, destination context.Context, ocPrefix, ocPath string) context.Context {
	nCtx, err := CopyLoggerContext(source, destination)
	if err != nil {
		zlog.Ctx(source).Error().Err(err).Msg("error rewrap logger")
		return destination
	}

	nCtx = FromContextLoggerZlog(nCtx).WrapZlog(source, ocPrefix, ocPath)
	zlog.Ctx(nCtx).UpdateContext(func(c zlog.Context) zlog.Context { return zlog.Ctx(source).With() })

	return nCtx
}

// WrapZlog создаёт zlog в контексте
func (l AppZlogLogger) WrapZlog(ctx context.Context, ocPrefix, ocPath string) context.Context {
	return l.zlogCreator(ocPrefix, ocPath).WithContext(ctx)
}

// ZerologUpdater implements reqctx.LoggerContextUpdater for zerolog
type ZerologUpdater struct{}

// NewZerologUpdater creates a new ZerologUpdater
func NewZerologUpdater() *ZerologUpdater {
	return &ZerologUpdater{}
}

// UpdateContext implements reqctx.LoggerContextUpdater
func (z *ZerologUpdater) UpdateContext(ctx context.Context, update func(c reqctx.LoggerContext) reqctx.LoggerContext) context.Context {
	logger := zlog.Ctx(ctx)
	logger.UpdateContext(func(c zlog.Context) zlog.Context {
		adapter := &zerologAdapter{ctx: c}
		updated := update(adapter)
		return updated.(*zerologAdapter).ctx
	})
	return logger.WithContext(ctx)
}

// zerologAdapter adapts zlog.Context to reqctx.LoggerContext
type zerologAdapter struct {
	ctx zlog.Context
}

func (z *zerologAdapter) Int64(key string, val int64) reqctx.LoggerContext {
	z.ctx = z.ctx.Int64(key, val)
	return z
}

func (z *zerologAdapter) Str(key string, val string) reqctx.LoggerContext {
	z.ctx = z.ctx.Str(key, val)
	return z
}

func (z *zerologAdapter) Time(key string, val time.Time) reqctx.LoggerContext {
	z.ctx = z.ctx.Time(key, val)
	return z
}
