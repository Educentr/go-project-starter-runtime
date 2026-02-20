package rest

import (
	"context"

	"github.com/go-faster/errors"

	"github.com/Educentr/go-project-starter-runtime/pkg/logger"
)

// Closer exists all router methods that can help you
// stop your http servers

func (s *Server) Shutdown(ctx context.Context) error {
	return s.ShutdownHTTP(ctx)
}

func (s *Server) GracefulStop(ctx context.Context) (<-chan struct{}, error) {
	stopped := make(chan struct{})

	go (func() {
		err := s.ShutdownHTTP(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.GetEventLogger().Error(ctx, "server shutdown with error", err)
		} else {
			logger.GetEventLogger().Warn(ctx, "server "+s.Name()+" successfully shutdown")
		}

		close(stopped)
	})()

	return stopped, nil
}

// ShutdownHTTP shutting down one of your http server
func (s *Server) ShutdownHTTP(ctx context.Context) error {
	s.shuttingDown.Store(true) // запретить рестарт
	s.mu.Lock()                // дождаться завершения текущего рестарта
	defer s.mu.Unlock()

	return s.httpSrv.Shutdown(ctx)
}
