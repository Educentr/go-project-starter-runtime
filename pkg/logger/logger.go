package logger

import (
	"context"
	"errors"
)

// AppLogger is the interface that logger wrappers must implement.
// Both zerolog and logrus implementations satisfy this interface.
type AppLogger interface {
	// WrapLogger creates a new logger instance in ctx using configuration.
	// The ocPrefix and ocPath parameters are used to configure the logger
	// (e.g., log level, format) from OnlineConf.
	WrapLogger(ctx context.Context, ocPrefix, ocPath string) context.Context

	// CopyFields copies logger-specific fields from the source context's logger
	// to the destination context's logger. Returns the updated destination context.
	CopyFields(source, destination context.Context) context.Context
}

var ErrLoggerNotImplementsInterface = errors.New("stored logger does not implement AppLogger interface")

// FromContext retrieves the AppLogger from context.
func FromContext(ctx context.Context) (AppLogger, error) {
	logI := ctx.Value(loggerContextKey)
	if logI == nil {
		return nil, ErrLoggerNotFoundInContext
	}

	appLogger, ok := logI.(AppLogger)
	if !ok {
		return nil, ErrLoggerNotImplementsInterface
	}

	return appLogger, nil
}

// ReWrap transfers logger from source context to destination context.
// It copies the logger wrapper, creates a new logger instance in the destination,
// and copies fields from the source logger to the destination logger.
func ReWrap(source, destination context.Context, ocPrefix, ocPath string) context.Context {
	nCtx, err := CopyLoggerContext(source, destination)
	if err != nil {
		return destination
	}

	appLogger, err := FromContext(nCtx)
	if err != nil {
		return destination
	}

	nCtx = appLogger.WrapLogger(nCtx, ocPrefix, ocPath)
	nCtx = appLogger.CopyFields(source, nCtx)

	return nCtx
}
