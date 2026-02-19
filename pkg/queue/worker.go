package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
)

const (
	defaultMaxProcessors = 1
	defaultBatchSize     = 10
	schedulerInterval    = 50 * time.Millisecond
)

// WorkerOption configures QueueWorker.
type WorkerOption func(*QueueWorker)

// WithMaxProcessors sets the maximum number of concurrent JobProcessors.
func WithMaxProcessors(n int) WorkerOption {
	return func(w *QueueWorker) {
		if n > 0 {
			w.maxProcessors = n
		}
	}
}

// WithBatchSize sets the number of tasks fetched per GetTask call.
func WithBatchSize(n int) WorkerOption {
	return func(w *QueueWorker) {
		if n > 0 {
			w.batchSize = n
		}
	}
}

// WithMetrics enables Prometheus metrics with the given registry and namespace.
func WithMetrics(registry *prometheus.Registry, namespace string) WorkerOption {
	return func(w *QueueWorker) {
		w.metrics = newQueueMetrics(registry, namespace)
	}
}

// QueueWorker manages a pool of JobProcessors and distributes queue
// assignments using round-robin scheduling.
type QueueWorker struct {
	storage       Storage
	queueIDs      []int
	handler       HandlerFunc
	maxProcessors int
	batchSize     int
	metrics       *queueMetrics

	processors     []*jobProcessor
	runningCount   atomic.Int32
	cancel         context.CancelFunc
	stopped        chan struct{}
	gracefulOnce   sync.Once
	gracefulDoneCh chan struct{}
}

// NewQueueWorker creates a new QueueWorker.
func NewQueueWorker(storage Storage, queueIDs []int, handler HandlerFunc, opts ...WorkerOption) *QueueWorker {
	w := &QueueWorker{
		storage:       storage,
		queueIDs:      queueIDs,
		handler:       handler,
		maxProcessors: defaultMaxProcessors,
		batchSize:     defaultBatchSize,
		stopped:       make(chan struct{}),
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Run starts the worker. It launches JobProcessors and schedules queue assignments.
func (w *QueueWorker) Run(ctx context.Context, errGr *errgroup.Group) {
	ctx, w.cancel = context.WithCancel(ctx)

	if w.metrics != nil {
		w.metrics.runningWorkers.Inc()
	}

	// Start job processors
	w.processors = make([]*jobProcessor, w.maxProcessors)
	for i := range w.processors {
		jp := newJobProcessor(i, w.storage, w.handler, w.metrics)
		w.processors[i] = jp

		errGr.Go(func() error {
			jp.run(ctx, w.batchSize, &w.runningCount)
			return nil
		})
	}

	// Start round-robin scheduler
	errGr.Go(func() error {
		w.scheduler(ctx)
		return nil
	})
}

// scheduler distributes queue assignments to processors using round-robin.
func (w *QueueWorker) scheduler(ctx context.Context) {
	defer func() {
		if w.metrics != nil {
			w.metrics.runningWorkers.Dec()
		}

		close(w.stopped)
	}()

	if len(w.queueIDs) == 0 {
		return
	}

	queueIdx := 0
	ticker := time.NewTicker(schedulerInterval)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Close all assignment channels to signal processors to stop
			for _, jp := range w.processors {
				close(jp.assignCh)
			}

			// Wait for all processors to finish
			for _, jp := range w.processors {
				<-jp.doneCh
			}

			return
		case <-ticker.C:
			for _, jp := range w.processors {
				queueNum := w.queueIDs[queueIdx%len(w.queueIDs)]
				queueIdx++

				select {
				case jp.assignCh <- queueNum:
				default:
					// Processor is busy, skip
				}
			}
		}
	}
}

// Shutdown performs a hard shutdown of the worker.
func (w *QueueWorker) Shutdown(_ context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}

	<-w.stopped

	return nil
}

// GracefulStop signals the worker to stop after completing current work.
// Returns a channel that is closed when all processors have finished.
func (w *QueueWorker) GracefulStop(_ context.Context) (<-chan struct{}, error) {
	ch := make(chan struct{})

	w.gracefulOnce.Do(func() {
		w.gracefulDoneCh = ch

		go func() {
			if w.cancel != nil {
				w.cancel()
			}

			<-w.stopped
			close(ch)
		}()
	})

	if w.gracefulDoneCh != nil {
		return w.gracefulDoneCh, nil
	}

	return ch, nil
}
