package reqctx

import (
	"context"
	"fmt"
	"time"

	"github.com/Educentr/go-onlineconf/pkg/onlineconf"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
)

type ctxKey int

const (
	actorField ctxKey = iota
	requestIDField
	requestStartTimeField
	metricHist
	metricCount
	processInfoField
)

var (
	ErrUndefinedActor            = fmt.Errorf("undefined actor")
	ErrCreateContext             = fmt.Errorf("failed to create request context")
	errEmptyRequestID            = fmt.Errorf("setRequestID in context error: RID is empty")
	errUndefinedRequestStartTime = fmt.Errorf("undefined RequestStartTime")
	errInvalidRequestStartTime   = fmt.Errorf("invalid RequestStartTime value type")
	errInvalidRequestID          = fmt.Errorf("invalid RequestID value type")
)

// CreateContextWithTimeout creates a request context with explicit timeout.
// Unlike CreateContext, it does not read timeout from onlineconf —
// the caller is responsible for resolving the timeout value.
//
// Note: This function does NOT wrap the logger - that's the responsibility of the calling code.
// The caller should wrap the logger after calling this function if needed.
//
// Returns error if onlineconf config cloning fails. Callers should handle this error
// appropriately (e.g., return 500 Internal Server Error in REST handlers).
func CreateContextWithTimeout(mainCtx, configCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	clonedCtx, err := onlineconf.Clone(configCtx, mainCtx)
	if err != nil {
		return mainCtx, func() {}, errors.Wrap(ErrCreateContext, err.Error())
	}

	resultCtx := clonedCtx
	var cancel context.CancelFunc = func() {}

	if timeout != 0 {
		resultCtx, cancel = context.WithTimeout(clonedCtx, timeout)
	}

	return resultCtx, func() {
		cancel()
		_ = onlineconf.Release(configCtx, clonedCtx)
	}, nil
}

// Deprecated: Use CreateContextWithTimeout instead. CreateContext reads timeout
// from onlineconf internally, which prevents proper fallback logic and validation.
// Will be removed in v0.15.0.
func CreateContext(mainCtx, configCtx context.Context, configPathPrefix, configPath string) (context.Context, context.CancelFunc, error) {
	// Get timeout from onlineconf before cloning
	ocDefaultPath := onlineconf.MakePath(configPathPrefix, "default/timeout")
	ocPath := onlineconf.MakePath(configPathPrefix, configPath, "timeout")

	timeoutDef, err := onlineconf.GetDuration(configCtx, ocDefaultPath, 0)
	if err != nil {
		return mainCtx, func() {}, errors.Wrapf(ErrCreateContext, "get default timeout from %s: %v", ocDefaultPath, err)
	}

	timeout, err := onlineconf.GetDuration(configCtx, ocPath, timeoutDef)
	if err != nil {
		return mainCtx, func() {}, errors.Wrapf(ErrCreateContext, "get timeout from %s: %v", ocPath, err)
	}

	// Clone onlineconf config from main context
	clonedCtx, err := onlineconf.Clone(configCtx, mainCtx)
	if err != nil {
		return mainCtx, func() {}, errors.Wrap(ErrCreateContext, err.Error())
	}

	// resultCtx will be wrapped with timeout if needed, but we keep clonedCtx
	// separately for onlineconf.Release which requires the original cloned context
	resultCtx := clonedCtx
	var cancel context.CancelFunc = func() {}

	if timeout != 0 {
		resultCtx, cancel = context.WithTimeout(clonedCtx, timeout)
	}

	return resultCtx, func() {
		cancel()
		_ = onlineconf.Release(configCtx, clonedCtx)
	}, nil
}

func GetActor(ctx context.Context) (ds.Actor, error) {
	ac := ctx.Value(actorField)
	if ac == nil {
		return nil, ErrUndefinedActor
	}

	curActor, ok := ac.(ds.Actor)
	if !ok {
		return nil, fmt.Errorf("invalid actor value type: `%T`", ac)
	}

	return curActor, nil
}

func SetActor(ctx context.Context, act ds.Actor) (context.Context, error) {
	if act.GetID() == 0 {
		return nil, fmt.Errorf("invalid actor: %v", act)
	}

	// Update logger context if updater is set
	if globalLoggerUpdater != nil {
		ctx = globalLoggerUpdater.UpdateContext(ctx, func(c LoggerContext) LoggerContext {
			return c.Int64("ActorUID", act.GetID())
		})
	}

	// Mark that actor was set in process info
	if pi, err := GetProcessInfo(ctx); err == nil && pi != nil {
		pi.ActorWasSet = true
	}

	return context.WithValue(ctx, actorField, act), nil
}

func GetRequestID(ctx context.Context) (string, error) {
	ridf := ctx.Value(requestIDField)
	if ridf == nil {
		return "", nil
	}

	rid, ok := ridf.(string)
	if !ok {
		return "", errors.Wrapf(errInvalidRequestID, "%T", ridf)
	}

	return rid, nil
}

func SetRequestID(ctx context.Context, rID string) (context.Context, error) {
	if rID == "" {
		return ctx, errEmptyRequestID
	}

	// Update logger context if updater is set
	if globalLoggerUpdater != nil {
		ctx = globalLoggerUpdater.UpdateContext(ctx, func(c LoggerContext) LoggerContext {
			return c.Str("RequestID", rID)
		})
	}

	return context.WithValue(ctx, requestIDField, rID), nil
}

func GetRequestStartTime(ctx context.Context) (time.Time, error) {
	t := ctx.Value(requestStartTimeField)
	if t == nil {
		return time.Time{}, errUndefinedRequestStartTime
	}

	tt, ok := t.(time.Time)
	if !ok {
		return time.Time{}, errors.Wrapf(errInvalidRequestStartTime, "%T", t)
	}

	return tt, nil
}

func SetRequestStartTime(ctx context.Context, time time.Time) context.Context {
	// Update logger context if updater is set
	if globalLoggerUpdater != nil {
		ctx = globalLoggerUpdater.UpdateContext(ctx, func(c LoggerContext) LoggerContext {
			return c.Time("RequestStartTime", time)
		})
	}

	return context.WithValue(ctx, requestStartTimeField, time)
}

func CreateCumulativeMetric(ctx context.Context, collectorHist *prometheus.HistogramVec, collectorCount *prometheus.CounterVec) (context.Context, error) {
	ctx = context.WithValue(ctx, metricHist, NewContextCumulativeMetric(collectorHist))
	return context.WithValue(ctx, metricCount, NewContextCumulativeMetric(collectorCount)), nil
}

func GetCumulativeMetric(ctx context.Context, metricType ctxKey) *ContextCumulativeMetric {
	ccmp := ctx.Value(metricType)
	if ccmp == nil {
		return nil
	}

	ccm, ok := ccmp.(*ContextCumulativeMetric)
	if !ok {
		// Library code doesn't log - just return nil
		return nil
	}

	return ccm
}

func IncCumulativeMetric(ctx context.Context, name string, diff int32) {
	if ccm := GetCumulativeMetric(ctx, metricCount); ccm != nil {
		ccm.IncMetric(name, diff)
	}
}

func TimeCumulativeMetric(ctx context.Context, name string, dur time.Duration) {
	if ccm := GetCumulativeMetric(ctx, metricHist); ccm != nil {
		ccm.TimeMetric(name, dur)
	}
}

func FlushCumulativeMetric(ctx context.Context, requestName string, labels ...string) {
	for _, mn := range []ctxKey{metricCount, metricHist} {
		if ccm := GetCumulativeMetric(ctx, mn); ccm != nil {
			ccm.FlushMetric(requestName, labels...)
		}
	}
}

// SetProcessInfo stores RequestProcessInfo in context
func SetProcessInfo(ctx context.Context, info *RequestProcessInfo) context.Context {
	return context.WithValue(ctx, processInfoField, info)
}

// GetProcessInfo retrieves RequestProcessInfo from context
func GetProcessInfo(ctx context.Context) (*RequestProcessInfo, error) {
	pii := ctx.Value(processInfoField)
	if pii == nil {
		return nil, errors.New("processInfo not found in context")
	}

	pi, ok := pii.(*RequestProcessInfo)
	if !ok {
		return nil, errors.New("invalid processInfo object")
	}

	return pi, nil
}
