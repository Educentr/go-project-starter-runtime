package queue

import (
	"context"
	"time"
)

// Storage is the interface for queue storage implementations.
// It provides atomic operations for task management with guaranteed delivery.
type Storage interface {
	// GetTask atomically takes up to count tasks from the given queue.
	// Taken tasks are marked as "in progress" and will not be returned
	// to other consumers until the visibility timeout expires.
	GetTask(ctx context.Context, queueNum int, count int) ([]Task, error)

	// DoneTask marks tasks as completed. They are removed from storage.
	DoneTask(ctx context.Context, taskIDs []int64) error

	// RetryTask returns tasks to the queue with the specified next start time.
	// Attempts is incremented, PrevStartTime is set to the current StartTime.
	RetryTask(ctx context.Context, retries []RetryInfo) error

	// PutTask adds a new task to the queue.
	PutTask(ctx context.Context, req PutTaskRequest) error

	// GetStat returns statistics for all queues.
	GetStat(ctx context.Context) (*Stats, error)
}

// Task is a task retrieved from storage.
type Task struct {
	ID            int64
	QueueNum      int
	Data          []byte
	Attempts      int
	CreatedAt     time.Time
	StartTime     time.Time
	PrevStartTime time.Time
}

// RetryInfo contains information for retrying a task.
type RetryInfo struct {
	TaskID        int64
	NextStartTime time.Time
}

// PutTaskRequest is a request to add a new task to the queue.
type PutTaskRequest struct {
	QueueNum  int
	Data      []byte
	StartTime *time.Time
	Delay     time.Duration
}

// QueueStat contains statistics for a single queue.
type QueueStat struct {
	QueueNum     int
	TotalTasks   int64
	ReadyTasks   int64
	TakenTasks   int64
	DelayedTasks int64
}

// Stats contains statistics for all queues.
type Stats struct {
	QueueStats map[int]QueueStat
}

// HandlerStats contains processing statistics returned by a handler.
type HandlerStats struct {
	Processed int
	Errors    int
	Retried   int
	Duration  time.Duration
	LastError error
}
