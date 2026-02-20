package mw

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Educentr/go-project-starter-runtime/pkg/logger"
	"github.com/Educentr/go-project-starter-runtime/pkg/reqctx"
)

// HitInfoCallback interface for async metrics processing.
type HitInfoCallback interface {
	HitInfo(ctx context.Context, method string, u *url.URL, status int, contentLength int, ip string, contentType string, userAgent string, referer string, execTime float64)
}

// HitInfoContextCreator creates a context for HitInfo callback.
// This abstracts away the OnlineConf-specific context creation.
type HitInfoContextCreator interface {
	CreateHitInfoContext(requestCtx, appCtx context.Context, urlPath string) (context.Context, context.CancelFunc, error)
}

// responseWriter wraps http.ResponseWriter to capture status code and response size.
// Replaces external negroni dependency.
type responseWriter struct {
	http.ResponseWriter
	status      int
	size        int
	wroteHeader bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	n, err := rw.ResponseWriter.Write(b)
	rw.size += n

	return n, err
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) Size() int {
	return rw.size
}

// HTTPServerMiddlewareMetrics creates a middleware that collects HTTP request metrics.
// Parameters:
// - serviceName: Main.Name (e.g., "my-api") - used for Prometheus namespace
// - transportName: Transport.Name (e.g., "api_v1") - the REST transport name
// - appName: Application.Name (e.g., "web-app") - for app-specific paths
// - hitInfoCtxCreator: creates context for HitInfo callback (nil to skip HitInfo context)
func HTTPServerMiddlewareMetrics(
	ctx context.Context,
	serviceName, transportName, appName string,
	metrics *prometheus.Registry,
	dwm HitInfoCallback,
	hitInfoCtxCreator HitInfoContextCreator,
) func(http.Handler) http.Handler {
	// Use serviceName for Prometheus namespace and transportName for subsystem (sanitized: replace - with _)
	// This ensures unique metric names when multiple REST transports exist
	// Metric name format: {namespace}_{subsystem}_{name}, e.g., my_api_api_v1_request_duration_seconds
	namespace := strings.ReplaceAll(serviceName, "-", "_")
	subsystem := strings.ReplaceAll(transportName, "-", "_")

	metricHTTP := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "request_duration_seconds",
		Help:      "The latency of the HTTP requests.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"handler", "method", "code"})
	metrics.MustRegister(metricHTTP)

	requestCumulativeMetricsHist := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cumulative_per_request_hist",
		Help:      "The cumulative latency metrics per request",
		Buckets:   prometheus.DefBuckets,
	}, []string{"requestName", "method", "code", "metric"})
	metrics.MustRegister(requestCumulativeMetricsHist)

	requestCumulativeMetricsCount := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cumulative_per_request_count",
		Help:      "The cumulative count metrics per request",
	}, []string{"requestName", "method", "code", "metric"})
	metrics.MustRegister(requestCumulativeMetricsCount)

	log := logger.GetEventLogger()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Info(r.Context(), "Start request",
				logger.Str("Method", r.Method),
				logger.Str("Content-Length", r.Header.Get("content-length")),
				logger.Str("X-Real-IP", r.Header.Get("x-real-ip")),
				logger.Str("Content-Type", r.Header.Get("content-type")),
				logger.Str("User-Agent", r.Header.Get("user-agent")),
				logger.Str("Referer", r.Header.Get("referer")),
			)

			StartedAt := time.Now()

			var err error

			workCtx, err := reqctx.CreateCumulativeMetric(r.Context(), requestCumulativeMetricsHist, requestCumulativeMetricsCount)
			if err != nil {
				log.Error(r.Context(), "can't init per request cumulative metrics", err)
			}

			rw := newResponseWriter(w)
			r = r.WithContext(workCtx)
			next.ServeHTTP(rw, r)

			// Check if request context deadline was exceeded (handler timeout fired)
			if deadline, ok := r.Context().Deadline(); ok && time.Now().After(deadline) {
				log.Warn(r.Context(), "Request handler timeout exceeded",
					logger.Str("Method", r.Method),
					logger.Str("Path", r.URL.Path),
					logger.Str("HandlerTimeout", deadline.Sub(StartedAt).String()),
					logger.Str("ExecTime", time.Since(StartedAt).String()),
				)
			}

			status := strconv.Itoa(rw.Status())

			reqctx.FlushCumulativeMetric(r.Context(), r.URL.Path, r.Method, status)

			execTime := time.Since(StartedAt).Seconds()
			metricHTTP.WithLabelValues(r.URL.Path, r.Method, status).Observe(execTime)

			userIP := ""

			posibleAddresses := []string{
				r.Header.Get("x-real-ip"),
				strings.Split(r.Header.Get("x-forwarded-for"), ",")[0],
				"0.0.0.0",
			}

			for _, ip := range posibleAddresses {
				if ip != "" {
					userIP = ip
					break
				}
			}

			if hitInfoCtxCreator != nil {
				mwCtx, cancel, hitInfoErr := hitInfoCtxCreator.CreateHitInfoContext(r.Context(), ctx, r.URL.Path)
				if hitInfoErr != nil {
					log.Error(r.Context(), "Failed to create context for HitInfo", hitInfoErr)
				} else {
					dwm.HitInfo(mwCtx, r.Method, r.URL, rw.Status(), rw.Size(), userIP, rw.Header().Get("Content-Type"), r.Header.Get("user-agent"), r.Header.Get("referer"), execTime)
					cancel()
				}
			} else {
				dwm.HitInfo(r.Context(), r.Method, r.URL, rw.Status(), rw.Size(), userIP, rw.Header().Get("Content-Type"), r.Header.Get("user-agent"), r.Header.Get("referer"), execTime)
			}

			statusCode := rw.Status()
			logFields := []logger.Field{
				logger.Int("Status", statusCode),
				logger.Str("Method", r.Method),
				logger.Int("Content-Length", rw.Size()),
				logger.Str("X-Real-IP", userIP),
				logger.Str("Content-Type", rw.Header().Get("Content-Type")),
				logger.Str("User-Agent", r.Header.Get("user-agent")),
				logger.Str("Referer", r.Header.Get("referer")),
			}

			if statusCode >= 500 {
				log.Error(r.Context(), "Done request", nil, logFields...)
			} else if statusCode >= 400 {
				log.Warn(r.Context(), "Done request", logFields...)
			} else {
				log.Info(r.Context(), "Done request", logFields...)
			}
		})
	}
}
