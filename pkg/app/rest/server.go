package rest

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
	"github.com/Educentr/go-project-starter-runtime/pkg/logger"
)

type Server struct {
	// Имя транспорта
	name string

	// HTTP сервер
	httpSrv *http.Server

	// Сервис с бизнес логикой приложения
	srv ds.IService

	// АПИ как презентационный уровень для сервиса
	router Router

	// Config provider for server configuration
	configProvider ServerConfigProvider

	// Для динамического обновления таймаутов.
	// Таймауты хранятся в atomic-полях, а не в http.Server напрямую,
	// т.к. net/http читает поля Server без синхронизации.
	// При изменении таймаутов — graceful restart http.Server.
	serviceName       string
	appName           string
	readTimeout       atomic.Int64 // наносекунды
	writeTimeout      atomic.Int64 // наносекунды
	idleTimeout       atomic.Int64 // наносекунды
	readHeaderTimeout atomic.Int64 // наносекунды

	// Для пересоздания сервера при изменении таймаутов
	mu           sync.Mutex      // защита httpSrv при замене
	errGr        *errgroup.Group // из Run(), для запуска нового сервера
	shuttingDown atomic.Bool     // true = финальный shutdown, не перезапускать
}

const (
	nameFieldLogger = "Name"
)

func (s *Server) Initialization(ctx context.Context) error {
	return nil
}

func NewServer(name string, router Router, configProvider ServerConfigProvider) *Server {
	return &Server{
		name:           name,
		router:         router,
		configProvider: configProvider,
	}
}

func (s *Server) Name() string {
	return s.name
}

func (s *Server) Init(ctx context.Context, serviceName, appName string, metrics *prometheus.Registry, srv ds.IService) error {
	var err error

	s.srv = srv
	s.serviceName = serviceName
	s.appName = appName

	cfg, err := s.configProvider.GetServerConfig(ctx, s.Name(), appName)
	if err != nil {
		return fmt.Errorf("error get server config: %w", err)
	}

	s.readTimeout.Store(int64(cfg.ReadTimeout))
	s.writeTimeout.Store(int64(cfg.WriteTimeout))
	s.idleTimeout.Store(int64(cfg.ReadTimeout))
	s.readHeaderTimeout.Store(int64(cfg.ReadHeaderTimeout))

	s.httpSrv = &http.Server{
		Addr:              cfg.IP + ":" + cfg.Port,
		WriteTimeout:      cfg.WriteTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		IdleTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	// Subscribe to timeout changes
	err = s.configProvider.SubscribeTimeoutChanges(ctx, s.Name(), appName, func() {
		s.updateTimeouts(ctx)
	})
	if err != nil {
		logger.GetEventLogger().Error(ctx, "failed to register timeout subscription", err)
	}

	err = s.router.InitRouters(ctx, s.httpSrv, s.srv, metrics)
	if err != nil {
		return errors.Wrap(err, "error initialization API Routers")
	}

	// GetMiddlewares parameters:
	// - appName: Application.Name (e.g., "web-app")
	// - serviceName: Main.Name (constant.ServiceName, e.g., "my-api")
	// - s.Name(): Transport.Name (e.g., "api_v1")
	mws, err := s.router.GetMiddlewares(ctx, appName, metrics, srv, serviceName, s.Name(), s.router.GetErrorHandler(), s.GetWriteTimeout)
	if err != nil {
		return errors.Wrap(err, "error initialization API middlewares")
	}

	for i := len(mws); i > 0; i-- {
		s.httpSrv.Handler = mws[i-1](s.httpSrv.Handler)
	}

	return nil
}

func (s *Server) updateTimeouts(ctx context.Context) {
	log := logger.GetEventLogger()

	cfg, err := s.configProvider.GetServerConfig(ctx, s.Name(), s.appName)
	if err != nil {
		log.Error(ctx, "failed to read updated server config", err)
		return
	}

	timeoutRead, timeoutWrite, headerTimeout := cfg.ReadTimeout, cfg.WriteTimeout, cfg.ReadHeaderTimeout

	// Сравниваем с текущими значениями
	changed := s.readTimeout.Load() != int64(timeoutRead) ||
		s.writeTimeout.Load() != int64(timeoutWrite) ||
		s.readHeaderTimeout.Load() != int64(headerTimeout)

	// Обновляем atomic-поля немедленно (middleware видит новые значения сразу)
	s.readTimeout.Store(int64(timeoutRead))
	s.writeTimeout.Store(int64(timeoutWrite))
	s.idleTimeout.Store(int64(timeoutRead))
	s.readHeaderTimeout.Store(int64(headerTimeout))

	log.Info(ctx, "REST server timeouts updated",
		logger.Str(nameFieldLogger, s.name),
		logger.Str("server_timeout_read", timeoutRead.String()),
		logger.Str("server_timeout_write", timeoutWrite.String()),
		logger.Str("server_timeout_read_header", headerTimeout.String()),
	)

	if changed && s.errGr != nil {
		go s.restartServer(ctx)
	}
}

// GetWriteTimeout returns the current server WriteTimeout for handler timeout validation.
func (s *Server) GetWriteTimeout() time.Duration {
	return time.Duration(s.writeTimeout.Load())
}

// GetReadTimeout returns the current server ReadTimeout.
func (s *Server) GetReadTimeout() time.Duration {
	return time.Duration(s.readTimeout.Load())
}

func (s *Server) Run(ctx context.Context, errGr *errgroup.Group) {
	log := logger.GetEventLogger()
	s.errGr = errGr

	log.Info(ctx, "Run rest server", logger.Str(nameFieldLogger, s.name))

	// initialization http server
	errGr.Go(func() error {
		log.Info(ctx, "server started serving",
			logger.Str("addr", s.httpSrv.Addr),
			logger.Str(nameFieldLogger, s.name),
		)

		err := s.httpSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(ctx, "server initialization/runtime error", err, logger.Str(nameFieldLogger, s.name))
			return err
		}

		return nil
	})
}

func (s *Server) restartServer(ctx context.Context) {
	log := logger.GetEventLogger()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.shuttingDown.Load() {
		return
	}

	newSrv := &http.Server{
		Addr:              s.httpSrv.Addr,
		Handler:           s.httpSrv.Handler,
		WriteTimeout:      time.Duration(s.writeTimeout.Load()),
		ReadTimeout:       time.Duration(s.readTimeout.Load()),
		IdleTimeout:       time.Duration(s.idleTimeout.Load()),
		ReadHeaderTimeout: time.Duration(s.readHeaderTimeout.Load()),
	}

	// Graceful shutdown старого сервера: закрывает listener, ждёт завершения активных соединений
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error(ctx, "server shutdown error during restart", err)
	}

	s.httpSrv = newSrv

	s.errGr.Go(func() error {
		log.Info(ctx, "server restarted with new timeouts",
			logger.Str("addr", newSrv.Addr),
			logger.Str(nameFieldLogger, s.name),
		)

		err := newSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(ctx, "server error after restart", err, logger.Str(nameFieldLogger, s.name))
			return err
		}

		return nil
	})
}
