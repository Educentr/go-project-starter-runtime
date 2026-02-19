package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestQueueWorker_ProcessesTasks(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")}))
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task2")}))

	var processed atomic.Int32
	handler := func(_ context.Context, storage Storage, _ int, tasks []Task) (HandlerStats, error) {
		ids := make([]int64, len(tasks))
		for i, t := range tasks {
			ids[i] = t.ID
		}
		processed.Add(int32(len(tasks)))
		_ = storage.DoneTask(ctx, ids)

		return HandlerStats{Processed: len(tasks)}, nil
	}

	w := NewQueueWorker(s, []int{1}, handler)

	errGr, gCtx := errgroup.WithContext(ctx)
	w.Run(gCtx, errGr)

	require.Eventually(t, func() bool {
		return processed.Load() >= 2
	}, 3*time.Second, 10*time.Millisecond)

	assert.NoError(t, w.Shutdown(ctx))
	assert.NoError(t, errGr.Wait())
}

func TestQueueWorker_RoundRobin(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("q1")}))
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 2, Data: []byte("q2")}))
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 3, Data: []byte("q3")}))

	var mu sync.Mutex
	seenQueues := make(map[int]int)

	handler := func(_ context.Context, storage Storage, queueNum int, tasks []Task) (HandlerStats, error) {
		mu.Lock()
		seenQueues[queueNum] += len(tasks)
		mu.Unlock()

		ids := make([]int64, len(tasks))
		for i, t := range tasks {
			ids[i] = t.ID
		}
		_ = storage.DoneTask(ctx, ids)

		return HandlerStats{Processed: len(tasks)}, nil
	}

	w := NewQueueWorker(s, []int{1, 2, 3}, handler)

	errGr, gCtx := errgroup.WithContext(ctx)
	w.Run(gCtx, errGr)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return seenQueues[1] >= 1 && seenQueues[2] >= 1 && seenQueues[3] >= 1
	}, 3*time.Second, 10*time.Millisecond)

	assert.NoError(t, w.Shutdown(ctx))
	assert.NoError(t, errGr.Wait())
}

func TestQueueWorker_GracefulStop(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task")}))

	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})

	handler := func(_ context.Context, storage Storage, _ int, tasks []Task) (HandlerStats, error) {
		close(handlerStarted)
		<-handlerDone

		ids := make([]int64, len(tasks))
		for i, t := range tasks {
			ids[i] = t.ID
		}
		_ = storage.DoneTask(ctx, ids)

		return HandlerStats{Processed: len(tasks)}, nil
	}

	w := NewQueueWorker(s, []int{1}, handler)

	errGr, gCtx := errgroup.WithContext(ctx)
	w.Run(gCtx, errGr)

	// Wait for handler to start
	select {
	case <-handlerStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not start")
	}

	// Initiate graceful stop
	doneCh, err := w.GracefulStop(ctx)
	require.NoError(t, err)

	// Should not be done yet (handler is blocked)
	select {
	case <-doneCh:
		t.Fatal("graceful stop completed while handler still running")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}

	// Unblock handler
	close(handlerDone)

	// Now graceful stop should complete
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("graceful stop did not complete")
	}

	assert.NoError(t, errGr.Wait())
}

func TestQueueWorker_MultipleProcessors(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numTasks = 20
	for i := 0; i < numTasks; i++ {
		require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task")}))
	}

	var processed atomic.Int32
	handler := func(_ context.Context, storage Storage, _ int, tasks []Task) (HandlerStats, error) {
		ids := make([]int64, len(tasks))
		for i, t := range tasks {
			ids[i] = t.ID
		}
		processed.Add(int32(len(tasks)))
		_ = storage.DoneTask(ctx, ids)

		return HandlerStats{Processed: len(tasks)}, nil
	}

	w := NewQueueWorker(s, []int{1}, handler, WithMaxProcessors(4), WithBatchSize(5))

	errGr, gCtx := errgroup.WithContext(ctx)
	w.Run(gCtx, errGr)

	require.Eventually(t, func() bool {
		return processed.Load() >= numTasks
	}, 3*time.Second, 10*time.Millisecond)

	assert.NoError(t, w.Shutdown(ctx))
	assert.NoError(t, errGr.Wait())
}

func TestQueueWorker_WithMetrics(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")}))

	var processed atomic.Int32
	handler := func(_ context.Context, storage Storage, _ int, tasks []Task) (HandlerStats, error) {
		processed.Add(int32(len(tasks)))
		ids := make([]int64, len(tasks))
		for i, t := range tasks {
			ids[i] = t.ID
		}
		_ = storage.DoneTask(ctx, ids)

		return HandlerStats{Processed: len(tasks)}, nil
	}

	registry := prometheus.NewRegistry()
	w := NewQueueWorker(s, []int{1}, handler, WithMetrics(registry, "test"))

	errGr, gCtx := errgroup.WithContext(ctx)
	w.Run(gCtx, errGr)

	require.Eventually(t, func() bool {
		return processed.Load() >= 1
	}, 3*time.Second, 10*time.Millisecond)

	// Verify metrics are registered
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, metricFamilies)

	assert.NoError(t, w.Shutdown(ctx))
	assert.NoError(t, errGr.Wait())
}
