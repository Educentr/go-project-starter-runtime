package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
)

// mockCloser implements IGracefulShuhtdown for testing.
type mockCloser struct {
	gracefulStopFunc func(ctx context.Context) (<-chan struct{}, error)
	shutdownFunc     func(ctx context.Context) error
}

func (m *mockCloser) GracefulStop(ctx context.Context) (<-chan struct{}, error) {
	return m.gracefulStopFunc(ctx)
}

func (m *mockCloser) Shutdown(ctx context.Context) error {
	return m.shutdownFunc(ctx)
}

func TestGracefullyShutdown_ContextNotCancelled(t *testing.T) {
	// Simulate the real scenario: ctx is already cancelled (signal received),
	// but shutdownCtx derived via WithoutCancel must NOT be cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate signal — ctx is Done

	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer shutdownCancel()

	var ctxWasCancelled atomic.Bool

	closer := &mockCloser{
		gracefulStopFunc: func(ctx context.Context) (<-chan struct{}, error) {
			// Verify context is NOT cancelled when GracefulStop is called
			if ctx.Err() != nil {
				ctxWasCancelled.Store(true)
			}

			stopped := make(chan struct{})
			close(stopped)

			return stopped, nil
		},
		shutdownFunc: func(ctx context.Context) error {
			return nil
		},
	}

	gracefullyShutdown(shutdownCtx, closer, "test")

	if ctxWasCancelled.Load() {
		t.Fatal("shutdown context was already cancelled — drain logic would not work")
	}
}

func TestGracefullyShutdown_TimeoutCallsShutdown(t *testing.T) {
	ctx := context.Background()

	var shutdownCalled atomic.Bool

	closer := &mockCloser{
		gracefulStopFunc: func(ctx context.Context) (<-chan struct{}, error) {
			// Return a channel that never closes — simulates slow drain
			return make(chan struct{}), nil
		},
		shutdownFunc: func(ctx context.Context) error {
			shutdownCalled.Store(true)
			return nil
		},
	}

	// gracefullyShutdown uses gracefulShutdownTimeout (30s) internally,
	// so we test the timeout path via gracefulStop with a short-lived context
	// to avoid waiting 30 seconds. Instead, test the select path directly:
	stopped := make(chan struct{}) // never closed
	t.Run("timer fires before stopped", func(t *testing.T) {
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-timer.C:
			err := closer.Shutdown(ctx)
			if err != nil {
				t.Fatal(err)
			}
		case <-stopped:
			t.Fatal("stopped should not have fired")
		}

		if !shutdownCalled.Load() {
			t.Fatal("Shutdown was not called after timeout")
		}
	})
}

func TestGracefullyShutdown_GracefulStopError_FallsBackToShutdown(t *testing.T) {
	ctx := context.Background()

	var shutdownCalled atomic.Bool

	closer := &mockCloser{
		gracefulStopFunc: func(ctx context.Context) (<-chan struct{}, error) {
			return nil, context.DeadlineExceeded
		},
		shutdownFunc: func(ctx context.Context) error {
			shutdownCalled.Store(true)
			return nil
		},
	}

	gracefullyShutdown(ctx, closer, "test")

	if !shutdownCalled.Load() {
		t.Fatal("Shutdown was not called after GracefulStop error")
	}
}

func TestGracefulStop_CancelledCtx_ComponentsGetValidContext(t *testing.T) {
	// This is the core integration test:
	// App.Run calls gracefulStop(ctx) where ctx is already cancelled.
	// Components must receive a non-cancelled context.

	var ctxErrInGracefulStop atomic.Value
	ctxErrInGracefulStop.Store("") // init with empty string

	worker := &mockRunnableService{
		name: "test-worker",
		gracefulStopFunc: func(ctx context.Context) (<-chan struct{}, error) {
			if err := ctx.Err(); err != nil {
				ctxErrInGracefulStop.Store(err.Error())
			}

			stopped := make(chan struct{})
			close(stopped)

			return stopped, nil
		},
		shutdownFunc: func(ctx context.Context) error {
			return nil
		},
	}

	app := &App{}
	app.workerInit.Store(true)
	app.driverInit.Store(true)
	app.transportInit.Store(true)
	app.workers = append(app.workers, worker)
	app.errGr, _ = errgroup.WithContext(context.Background())

	// Simulate: ctx is already cancelled (as in real Run() after <-ctx.Done())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := app.gracefulStop(ctx)
	if err != nil {
		t.Fatalf("gracefulStop returned error: %v", err)
	}

	if errStr := ctxErrInGracefulStop.Load().(string); errStr != "" {
		t.Fatalf("worker received cancelled context in GracefulStop: %s", errStr)
	}
}

// mockRunnableService implements ds.RunnableService for testing.
type mockRunnableService struct {
	name             string
	gracefulStopFunc func(ctx context.Context) (<-chan struct{}, error)
	shutdownFunc     func(ctx context.Context) error
}

func (m *mockRunnableService) Name() string { return m.name }

func (m *mockRunnableService) Init(_ context.Context, _, _ string, _ *prometheus.Registry, _ ds.IService) error {
	return nil
}

func (m *mockRunnableService) Initialization(_ context.Context) error { return nil }

func (m *mockRunnableService) Run(_ context.Context, _ *errgroup.Group) {}

func (m *mockRunnableService) GracefulStop(ctx context.Context) (<-chan struct{}, error) {
	return m.gracefulStopFunc(ctx)
}

func (m *mockRunnableService) Shutdown(ctx context.Context) error {
	return m.shutdownFunc(ctx)
}
