package logger

import (
	"context"

	zlog "github.com/rs/zerolog"
)

// ZerologEventLogger implements EventLogger using zerolog.
type ZerologEventLogger struct{}

// NewZerologEventLogger creates a new ZerologEventLogger.
func NewZerologEventLogger() *ZerologEventLogger {
	return &ZerologEventLogger{}
}

func (z *ZerologEventLogger) Error(ctx context.Context, msg string, err error, fields ...Field) {
	e := zlog.Ctx(ctx).Error()
	if err != nil {
		e = e.Err(err)
	}

	applyFieldsZerolog(e, fields)
	e.Msg(msg)
}

func (z *ZerologEventLogger) Warn(ctx context.Context, msg string, fields ...Field) {
	e := zlog.Ctx(ctx).Warn()
	applyFieldsZerolog(e, fields)
	e.Msg(msg)
}

func (z *ZerologEventLogger) Info(ctx context.Context, msg string, fields ...Field) {
	e := zlog.Ctx(ctx).Info()
	applyFieldsZerolog(e, fields)
	e.Msg(msg)
}

func applyFieldsZerolog(e *zlog.Event, fields []Field) {
	for _, f := range fields {
		switch v := f.Value.(type) {
		case string:
			e.Str(f.Key, v)
		case int:
			e.Int(f.Key, v)
		case int64:
			e.Int64(f.Key, v)
		default:
			e.Interface(f.Key, v)
		}
	}
}
