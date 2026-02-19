package queue

import (
	"context"
	"sync/atomic"
	"time"
)

// HandlerFunc is the function signature for processing tasks from a queue.
// It receives the storage, queue number, and a batch of tasks.
// The handler is responsible for calling DoneTask/RetryTask on the storage.
type HandlerFunc func(ctx context.Context, storage Storage, queueNum int, tasks []Task) (HandlerStats, error)

type jobProcessor struct {
	id       int
	storage  Storage
	handler  HandlerFunc
	metrics  *queueMetrics
	assignCh chan int // receives queue assignments
	doneCh   chan struct{}
}

func newJobProcessor(id int, storage Storage, handler HandlerFunc, metrics *queueMetrics) *jobProcessor {
	return &jobProcessor{
		id:       id,
		storage:  storage,
		handler:  handler,
		metrics:  metrics,
		assignCh: make(chan int, 1),
		doneCh:   make(chan struct{}),
	}
}

func (jp *jobProcessor) run(ctx context.Context, batchSize int, running *atomic.Int32) {
	defer close(jp.doneCh)

	running.Add(1)
	defer running.Add(-1)

	if jp.metrics != nil {
		jp.metrics.runningJobProcessors.Inc()
		defer jp.metrics.runningJobProcessors.Dec()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case queueNum, ok := <-jp.assignCh:
			if !ok {
				return
			}

			jp.processQueue(ctx, queueNum, batchSize)
		}
	}
}

func (jp *jobProcessor) processQueue(ctx context.Context, queueNum int, batchSize int) {
	start := time.Now()

	tasks, err := jp.storage.GetTask(ctx, queueNum, batchSize)
	if err != nil || len(tasks) == 0 {
		return
	}

	if jp.metrics != nil {
		jp.metrics.tasksInProgress.WithLabelValues(queueNumLabel(queueNum)).Add(float64(len(tasks)))
		defer func() {
			jp.metrics.tasksInProgress.WithLabelValues(queueNumLabel(queueNum)).Sub(float64(len(tasks)))
		}()
	}

	stats, handlerErr := jp.handler(ctx, jp.storage, queueNum, tasks)

	if jp.metrics != nil {
		label := queueNumLabel(queueNum)
		jp.metrics.cycleDuration.WithLabelValues(label).Observe(time.Since(start).Seconds())

		if stats.Processed > 0 {
			jp.metrics.tasksProcessedTotal.WithLabelValues(label).Add(float64(stats.Processed))
		}

		if stats.Errors > 0 {
			jp.metrics.tasksErrorsTotal.WithLabelValues(label).Add(float64(stats.Errors))
		}

		if handlerErr != nil {
			jp.metrics.handlerErrorsTotal.WithLabelValues(label).Inc()
		}
	}
}
