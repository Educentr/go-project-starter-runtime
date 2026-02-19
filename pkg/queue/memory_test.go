package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStorage_PutAndGetTask(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("hello")})
	require.NoError(t, err)

	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, []byte("hello"), tasks[0].Data)
	assert.Equal(t, 1, tasks[0].QueueNum)
	assert.Equal(t, 0, tasks[0].Attempts)
}

func TestMemoryStorage_GetTask_Atomicity(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")})
	require.NoError(t, err)

	// First consumer gets the task
	tasks1, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks1, 1)

	// Second consumer gets nothing (task is taken)
	tasks2, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	assert.Empty(t, tasks2)
}

func TestMemoryStorage_DoneTask(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")})
	require.NoError(t, err)

	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	err = s.DoneTask(ctx, []int64{tasks[0].ID})
	require.NoError(t, err)

	// Task should be removed from storage
	stats, err := s.GetStat(ctx)
	require.NoError(t, err)
	assert.Empty(t, stats.QueueStats)
}

func TestMemoryStorage_RetryTask(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")})
	require.NoError(t, err)

	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	origStartTime := tasks[0].StartTime

	// Retry with next start time in the past (immediately available)
	nextStart := time.Now().Add(-time.Second)
	err = s.RetryTask(ctx, []RetryInfo{{TaskID: tasks[0].ID, NextStartTime: nextStart}})
	require.NoError(t, err)

	// Task should be available again with updated attempts
	tasks, err = s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, 1, tasks[0].Attempts)
	assert.Equal(t, origStartTime, tasks[0].PrevStartTime)
}

func TestMemoryStorage_RetryTask_Delayed(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")})
	require.NoError(t, err)

	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	// Retry with future start time
	nextStart := time.Now().Add(time.Hour)
	err = s.RetryTask(ctx, []RetryInfo{{TaskID: tasks[0].ID, NextStartTime: nextStart}})
	require.NoError(t, err)

	// Task should NOT be available (delayed)
	tasks, err = s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	assert.Empty(t, tasks)

	// Stats should show delayed task
	stats, err := s.GetStat(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.QueueStats[1].DelayedTasks)
}

func TestMemoryStorage_PutTask_WithStartTime(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	futureTime := time.Now().Add(time.Hour)
	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("delayed"), StartTime: &futureTime})
	require.NoError(t, err)

	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestMemoryStorage_PutTask_WithDelay(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("delayed"), Delay: time.Hour})
	require.NoError(t, err)

	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestMemoryStorage_VisibilityTimeout(t *testing.T) {
	s := NewMemoryStorage(WithVisibilityTimeout(100 * time.Millisecond))
	defer s.Close()

	ctx := context.Background()

	err := s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task1")})
	require.NoError(t, err)

	// Take the task
	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, 0, tasks[0].Attempts)

	// Wait for visibility timeout + reaper cycle
	time.Sleep(1200 * time.Millisecond)

	// Task should be returned to queue
	tasks, err = s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, 1, tasks[0].Attempts)
	assert.False(t, tasks[0].PrevStartTime.IsZero())
}

func TestMemoryStorage_GetStat(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	// Add tasks to different queues
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("a")}))
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("b")}))
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 2, Data: []byte("c")}))

	futureTime := time.Now().Add(time.Hour)
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("d"), StartTime: &futureTime}))

	// Take one task from queue 1
	tasks, err := s.GetTask(ctx, 1, 1)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	stats, err := s.GetStat(ctx)
	require.NoError(t, err)

	q1 := stats.QueueStats[1]
	assert.Equal(t, int64(3), q1.TotalTasks)
	assert.Equal(t, int64(1), q1.ReadyTasks)
	assert.Equal(t, int64(1), q1.TakenTasks)
	assert.Equal(t, int64(1), q1.DelayedTasks)

	q2 := stats.QueueStats[2]
	assert.Equal(t, int64(1), q2.TotalTasks)
	assert.Equal(t, int64(1), q2.ReadyTasks)
}

func TestMemoryStorage_MultipleQueues(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()

	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("q1")}))
	require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 2, Data: []byte("q2")}))

	tasks, err := s.GetTask(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, []byte("q1"), tasks[0].Data)

	tasks, err = s.GetTask(ctx, 2, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, []byte("q2"), tasks[0].Data)
}

func TestMemoryStorage_ConcurrentAccess(t *testing.T) {
	s := NewMemoryStorage()
	defer s.Close()

	ctx := context.Background()
	const numTasks = 100

	for i := 0; i < numTasks; i++ {
		require.NoError(t, s.PutTask(ctx, PutTaskRequest{QueueNum: 1, Data: []byte("task")}))
	}

	var mu sync.Mutex
	var allTasks []Task

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				tasks, err := s.GetTask(ctx, 1, 5)
				if err != nil {
					t.Error(err)
					return
				}

				if len(tasks) == 0 {
					return
				}

				mu.Lock()
				allTasks = append(allTasks, tasks...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Each task should be taken exactly once
	assert.Equal(t, numTasks, len(allTasks))

	seen := make(map[int64]struct{})
	for _, task := range allTasks {
		_, exists := seen[task.ID]
		assert.False(t, exists, "task %d taken more than once", task.ID)
		seen[task.ID] = struct{}{}
	}
}
