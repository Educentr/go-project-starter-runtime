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
	errEmptyRequestID            = fmt.Errorf("setRequestID in context error: RID is empty")
	errUndefinedRequestStartTime = fmt.Errorf("undefined RequestStartTime")
	errInvalidRequestStartTime   = fmt.Errorf("invalid RequestStartTime value type")
	errInvalidRequestID          = fmt.Errorf("invalid RequestID value type")
)

// CreateContext creates a new context with cloned onlineconf config and timeout.
// Note: This function does NOT wrap the logger - that's the responsibility of the calling code.
// The caller should wrap the logger after calling this function if needed.
func CreateContext(mainCtx, configCtx context.Context, configPathPrefix, configPath string) (context.Context, context.CancelFunc) {
	// Clone onlineconf config from main context
	cpCtx, err := onlineconf.Clone(configCtx, mainCtx)
	if err != nil {
		// Return main context on error (library code doesn't log)
		return mainCtx, func() {}
	}

	pCtx := cpCtx

	// Get timeout from onlineconf
	ocDefaultPath := onlineconf.MakePath(configPathPrefix, "default/timeout")
	ocPath := onlineconf.MakePath(configPathPrefix, configPath, "timeout")

	timeoutDef, _ := onlineconf.GetDuration(pCtx, ocDefaultPath, 0)
	timeout, _ := onlineconf.GetDuration(pCtx, ocPath, timeoutDef)

	var cancel context.CancelFunc = func() {}

	if timeout != 0 {
		pCtx, cancel = context.WithTimeout(pCtx, timeout)
	}

	return pCtx, func() {
		cancel()
		_ = onlineconf.Release(configCtx, cpCtx)
	}
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
