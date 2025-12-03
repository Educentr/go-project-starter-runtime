package app

import (
	"context"
	"errors"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

type IGracefulShuhtdown interface {
	GracefulStop(ctx context.Context) (<-chan struct{}, error)
	Shutdown(ctx context.Context) error
}

const (
	gracefulShutdownTimeout = 30 * time.Second
	loggerObjectName        = "object"
)

func gracefullyShutdown(shutdownCtx context.Context, closer IGracefulShuhtdown, name string) {
	stopped, err := closer.GracefulStop(shutdownCtx)
	if err != nil {
		err = closer.Shutdown(shutdownCtx)
		if err != nil {
			// Library code doesn't log errors - just try to shutdown
		}

		return
	}

	// hard limit
	t := time.NewTimer(gracefulShutdownTimeout)
	select {
	case <-t.C:
		err = closer.Shutdown(shutdownCtx)
		if err != nil {
			// Library code doesn't log errors
		}
	case <-stopped:
		t.Stop()
	}
}

func (a *App) InitGracefulStop(ctx context.Context) context.Context {
	// graceful shutdown
	// Note: SIGKILL cannot be caught in Unix, so we only listen for SIGINT and SIGTERM
	ctx, a.ctxStop = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	// init error group
	a.errGr, ctx = errgroup.WithContext(ctx)

	return ctx
}

func (a *App) gracefulStop(ctx context.Context) error {
	var err error

	a.ready.Store(false) // помечаем, что приложение не готово принимать запросы

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, gracefulShutdownTimeout)
	defer shutdownCancel()

	if len(a.transports) > 0 {
		for _, transport := range a.transports {
			gracefullyShutdown(shutdownCtx, transport, "Transport "+transport.Name())
		}
	}

	if len(a.workers) > 0 {
		for _, worker := range a.workers {
			gracefullyShutdown(shutdownCtx, worker, "Worker "+worker.Name())
		}
	}

	if len(a.drivers) > 0 {
		for _, driver := range a.drivers {
			gracefullyShutdown(shutdownCtx, driver, "Driver "+driver.Name())
		}
	}

	err = a.errGr.Wait()
	if errors.Is(err, context.Canceled) {
		err = nil
	}

	return err
}
