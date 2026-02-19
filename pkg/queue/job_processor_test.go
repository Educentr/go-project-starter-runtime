package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobProcessor_ProcessesQueue(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")}))

	var processed atomic.Int32
	handler := func(_ context.Context, storage Storage, queueNum int, tasks []Task) (HandlerStats, error) {
		processed.Add(int32(len(tasks)))
		_ = storage.DoneTask(ctx, []int64{tasks[0].ID})

		return HandlerStats{Processed: len(tasks)}, nil
	}

	jp := newJobProcessor(0, s, handler, nil)

	var running atomic.Int32
	go jp.run(ctx, 10, &running)

	// Send assignment
	jp.assignCh <- 1

	// Wait for processing
	require.Eventually(t, func() bool {
		return processed.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	assert.Equal(t, int32(1), processed.Load())
	cancel()
	<-jp.doneCh
}

func TestJobProcessor_SkipsEmptyQueue(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var handlerCalled atomic.Int32
	handler := func(_ context.Context, _ Storage, _ int, _ []Task) (HandlerStats, error) {
		handlerCalled.Add(1)
		return HandlerStats{}, nil
	}

	jp := newJobProcessor(0, s, handler, nil)

	var running atomic.Int32
	go jp.run(ctx, 10, &running)

	// Assign empty queue
	jp.assignCh <- 1
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(0), handlerCalled.Load())
	cancel()
	<-jp.doneCh
}
