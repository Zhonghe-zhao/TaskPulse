package store

import (
	"context"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
)

type TaskStatsStore interface {
	SnapshotTaskStats(ctx context.Context, now time.Time) (*TaskStatsSnapshot, error)
}

type TaskStatsSnapshot struct {
	StatusCounts       map[domain.TaskStatus]int
	AvailableCounts    map[domain.TaskStatus]int
	OldestAvailableAge map[domain.TaskStatus]time.Duration
}

func NewTaskStatsSnapshot() *TaskStatsSnapshot {
	return &TaskStatsSnapshot{
		StatusCounts:       make(map[domain.TaskStatus]int),
		AvailableCounts:    make(map[domain.TaskStatus]int),
		OldestAvailableAge: make(map[domain.TaskStatus]time.Duration),
	}
}

func (s *MemoryTaskStore) SnapshotTaskStats(ctx context.Context, now time.Time) (*TaskStatsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := NewTaskStatsSnapshot()
	for _, task := range s.tasks {
		snapshot.StatusCounts[task.Status]++
		if isAvailableTask(task, now) {
			snapshot.AvailableCounts[task.Status]++
			age := now.Sub(task.AvailableAt)
			if previous, exists := snapshot.OldestAvailableAge[task.Status]; !exists || age > previous {
				snapshot.OldestAvailableAge[task.Status] = age
			}
		}
	}
	return snapshot, nil
}

func isAvailableTask(task *domain.Task, now time.Time) bool {
	if task == nil {
		return false
	}
	if task.Status != domain.TaskStatusQueued && task.Status != domain.TaskStatusRetrying {
		return false
	}
	return !task.AvailableAt.After(now)
}
