package queue

import (
	"context"
	"sync"
	"time"
)

const defaultVisibilityTimeout = 60 * time.Second

// Option configures MemoryStorage.
type Option func(*MemoryStorage)

// WithVisibilityTimeout sets the visibility timeout for taken tasks.
// If a task is not completed or retried within this duration, it is
// automatically returned to the queue with Attempts incremented.
func WithVisibilityTimeout(d time.Duration) Option {
	return func(s *MemoryStorage) {
		s.visibilityTimeout = d
	}
}

type memTask struct {
	id            int64
	queueNum      int
	data          []byte
	attempts      int
	createdAt     time.Time
	startTime     time.Time
	prevStartTime time.Time
	taken         bool
	takenAt       time.Time
}

// MemoryStorage is an in-memory implementation of Storage.
// It is intended for development and testing. Data is lost on restart.
type MemoryStorage struct {
	mu                sync.Mutex
	tasks             map[int64]*memTask
	nextID            int64
	visibilityTimeout time.Duration

	done chan struct{}
	wg   sync.WaitGroup
}

// NewMemoryStorage creates a new in-memory storage.
func NewMemoryStorage(opts ...Option) *MemoryStorage {
	s := &MemoryStorage{
		tasks:             make(map[int64]*memTask),
		nextID:            1,
		visibilityTimeout: defaultVisibilityTimeout,
		done:              make(chan struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.wg.Add(1)

	go s.reaper()

	return s
}

// Close stops the background goroutine that returns expired tasks.
func (s *MemoryStorage) Close() {
	close(s.done)
	s.wg.Wait()
}

// reaper periodically checks for tasks that have exceeded the visibility timeout
// and returns them to the queue.
func (s *MemoryStorage) reaper() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for _, t := range s.tasks {
				if t.taken && now.Sub(t.takenAt) >= s.visibilityTimeout {
					t.taken = false
					t.attempts++
					t.prevStartTime = t.startTime
					t.startTime = now
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *MemoryStorage) GetTask(_ context.Context, queueNum int, count int) ([]Task, error) {
	now := time.Now()
	result := make([]Task, 0, count)

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, t := range s.tasks {
		if len(result) >= count {
			break
		}

		if t.queueNum != queueNum || t.taken {
			continue
		}

		// Skip delayed tasks
		if t.startTime.After(now) {
			continue
		}

		t.taken = true
		t.takenAt = now

		result = append(result, Task{
			ID:            t.id,
			QueueNum:      t.queueNum,
			Data:          t.data,
			Attempts:      t.attempts,
			CreatedAt:     t.createdAt,
			StartTime:     t.startTime,
			PrevStartTime: t.prevStartTime,
		})
	}

	return result, nil
}

func (s *MemoryStorage) DoneTask(_ context.Context, taskIDs []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range taskIDs {
		delete(s.tasks, id)
	}

	return nil
}

func (s *MemoryStorage) RetryTask(_ context.Context, retries []RetryInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range retries {
		t, ok := s.tasks[r.TaskID]
		if !ok {
			continue
		}

		t.taken = false
		t.attempts++
		t.prevStartTime = t.startTime
		t.startTime = r.NextStartTime
	}

	return nil
}

func (s *MemoryStorage) PutTask(_ context.Context, req PutTaskRequest) error {
	now := time.Now()
	startTime := now

	if req.StartTime != nil {
		startTime = *req.StartTime
	} else if req.Delay > 0 {
		startTime = now.Add(req.Delay)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++

	s.tasks[id] = &memTask{
		id:        id,
		queueNum:  req.QueueNum,
		data:      req.Data,
		createdAt: now,
		startTime: startTime,
	}

	return nil
}

func (s *MemoryStorage) GetStat(_ context.Context) (*Stats, error) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	queueStats := make(map[int]QueueStat)

	for _, t := range s.tasks {
		qs := queueStats[t.queueNum]
		qs.QueueNum = t.queueNum
		qs.TotalTasks++

		switch {
		case t.taken:
			qs.TakenTasks++
		case t.startTime.After(now):
			qs.DelayedTasks++
		default:
			qs.ReadyTasks++
		}

		queueStats[t.queueNum] = qs
	}

	return &Stats{QueueStats: queueStats}, nil
}
