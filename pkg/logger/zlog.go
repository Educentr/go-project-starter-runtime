package logger

import (
	"context"

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
