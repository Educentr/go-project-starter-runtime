package mw

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Educentr/go-project-starter-runtime/pkg/app/rest"
	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
	"github.com/Educentr/go-project-starter-runtime/pkg/logger"
	"github.com/Educentr/go-project-starter-runtime/pkg/reqctx"
)

type DefaultMiddlewares struct {
	// HandlerTimeoutProvider resolves per-handler timeouts.
	HandlerTimeoutProvider rest.HandlerTimeoutProvider

	// SecurityConfigProvider manages auth/CSRF toggle.
	SecurityConfigProvider rest.SecurityConfigProvider

	// HitInfoContextCreatorFunc creates a HitInfoContextCreator for the given transport params.
	// If nil, HitInfo is called with the request context directly.
	HitInfoContextCreatorFunc func(transportName, appName string) HitInfoContextCreator

	// TransportConfigPath returns the OnlineConf path for transport handler configuration.
	// Used for logger re-wrapping per request. If nil, logger re-wrapping is skipped.
	TransportConfigPath func(transportName, appName string) string
}

type EmptyMiddlewares struct{}

// GetMiddlewares returns middleware chain for REST server.
// Parameters:
// - appName: Application.Name (e.g., "web-app") - the specific application instance
// - serviceName: Main.Name (e.g., "my-api") - the service-level name (constant.ServiceName)
// - transportName: Transport.Name (e.g., "api_v1") - the REST transport name
// - getWriteTimeout: returns current server WriteTimeout for handler timeout validation
func (dmw *DefaultMiddlewares) GetMiddlewares(
	ctx context.Context,
	appName string,
	metrics *prometheus.Registry,
	srv ds.IService,
	serviceName, transportName string,
	errHdl rest.RestErrorHandler,
	getWriteTimeout func() time.Duration,
) (
	[]func(next http.Handler) http.Handler,
	error,
) {
	log := logger.GetEventLogger()

	// Create HitInfo context creator if factory is provided
	var hitInfoCreator HitInfoContextCreator
	if dmw.HitInfoContextCreatorFunc != nil {
		hitInfoCreator = dmw.HitInfoContextCreatorFunc(transportName, appName)
	}

	// Мидлвари работают в том порядке в котором они находятся в этом массиве
	// Изменение порядка может сильно повлиять на работу сервиса, делать это надо только если вы уверенны что происходит
	mws := []func(next http.Handler) http.Handler{
		// Создаём контекст запроса
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Combine method + path for config key (e.g., GET_payment__token)
				pathKey := r.Method + "_" + strings.ReplaceAll(strings.TrimPrefix(r.URL.Path, "/"), "/", "__")

				// Resolve handler timeout via provider
				handlerTimeout := time.Duration(0)
				if dmw.HandlerTimeoutProvider != nil {
					handlerTimeout = dmw.HandlerTimeoutProvider.ResolveHandlerTimeout(ctx, transportName, appName, pathKey)
				}

				// Validate: handler timeout must be <= write timeout
				writeTimeout := getWriteTimeout()
				if writeTimeout > 0 && handlerTimeout > writeTimeout {
					log.Warn(r.Context(), "handler timeout exceeds write timeout, clamping",
						logger.Str("handler_timeout", handlerTimeout.String()),
						logger.Str("write_timeout", writeTimeout.String()),
						logger.Str("path", r.URL.Path),
					)
					handlerTimeout = writeTimeout
				}

				// r.Context() - контекст запроса (основной), ctx - контекст приложения (для onlineconf)
				reqCtx, cancel, err := reqctx.CreateContextWithTimeout(r.Context(), ctx, handlerTimeout)
				if err != nil {
					log.Error(r.Context(), "Failed to create request context", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				// Re-wrap logger from app context to request context
				if dmw.TransportConfigPath != nil {
					ocPrefix := dmw.TransportConfigPath(transportName, appName)
					reqCtx = logger.ReWrap(ctx, reqCtx, ocPrefix, pathKey)
				}

				// Enrich logger context with request info
				if updater := reqctx.GetLoggerUpdater(); updater != nil {
					reqCtx = updater.UpdateContext(reqCtx, func(c reqctx.LoggerContext) reqctx.LoggerContext {
						return c.Str("Method", r.Method).Str("URI", r.RequestURI)
					})
				}

				next.ServeHTTP(w, r.WithContext(reqCtx))
				cancel()
			})
		},
		PanicRecoveryMiddleware(ctx),                                                                      // Обработка паник
		HTTPServerMiddlewareRequestStartTime(),                                                            // Обогащаем контекст временем начала запроса
		HTTPServerMiddlewareTracing(ctx),                                                                  // Обогащаем контекст запроса ID для трейсинга
		HTTPServerMiddlewareMetrics(ctx, serviceName, transportName, appName, metrics, srv, hitInfoCreator), // Инициализируем метрики
		HTTPServerMiddlewareXServerHeader(ctx, appName),                                                   // Обогащаем ответ заголовком X-Server (с appName)
	}

	if dmw.SecurityConfigProvider != nil {
		httpAuthEnable, err := dmw.SecurityConfigProvider.IsHTTPAuthEnabled(ctx)
		if err != nil {
			return nil, fmt.Errorf("error get httpAuth enabled: %w", err)
		}

		if httpAuthEnable {
			mws = append(mws, HTTPServerMiddlewareAuth(ctx, srv.GetAuthorizer(), errHdl))
		}

		csrfEnabled, err := dmw.SecurityConfigProvider.IsCSRFEnabled(ctx)
		if err != nil {
			return nil, fmt.Errorf("error get csrf enabled: %w", err)
		}

		if csrfEnabled {
			mws = append(mws, HTTPServerMiddlewareCSRF(ctx, srv.GetAuthorizer(), errHdl))
		}
	}

	return mws, nil
}

func (emw *EmptyMiddlewares) GetMiddlewares(
	_ context.Context,
	_ string,
	_ *prometheus.Registry,
	_ ds.IService,
	_, _ string,
	_ rest.RestErrorHandler,
	_ func() time.Duration,
) (
	[]func(next http.Handler) http.Handler,
	error,
) {
	return []func(next http.Handler) http.Handler{}, nil
}

func PanicRecoveryMiddleware(ctx context.Context) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.GetEventLogger().Error(r.Context(), "Panic recovered", nil,
						logger.Str("Stack", strings.Replace(strings.Replace(string(debug.Stack()), "\n\t/", " --> /", -1), "\n", " => ", -1)),
						logger.Any("Error", err),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
