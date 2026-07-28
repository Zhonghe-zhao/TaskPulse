package mysqlstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	platformdb "github.com/zhaozhonghe/taskpulse/internal/platform/database"
	storeerrors "github.com/zhaozhonghe/taskpulse/internal/store"
)

func TestMySQLTaskCreationStoreCommitsTaskAndEventIntegration(t *testing.T) {
	if os.Getenv("TASKPULSE_MYSQL_INTEGRATION") != "1" {
		t.Skip("set TASKPULSE_MYSQL_INTEGRATION=1 to run MySQL store integration tests")
	}

	config, err := platformdb.MySQLConfigFromEnv()
	if err != nil {
		t.Fatalf("MySQLConfigFromEnv returned error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := platformdb.OpenMySQL(ctx, config)
	if err != nil {
		t.Fatalf("OpenMySQL returned error: %v", err)
	}
	defer db.Close()

	taskStore, err := NewTaskStore(db)
	if err != nil {
		t.Fatalf("NewTaskStore returned error: %v", err)
	}
	eventStore, err := NewEventStore(db)
	if err != nil {
		t.Fatalf("NewEventStore returned error: %v", err)
	}
	creator, err := NewTaskCreationStore(db)
	if err != nil {
		t.Fatalf("NewTaskCreationStore returned error: %v", err)
	}

	suffix := time.Now().UnixNano()
	taskID := fmt.Sprintf("task_atomic_%d", suffix)
	eventID := fmt.Sprintf("event_atomic_%d", suffix)
	rollbackTaskID := fmt.Sprintf("task_atomic_rollback_%d", suffix)
	t.Cleanup(func() {
		_, _ = db.ExecContext(
			context.Background(),
			"DELETE FROM tasks WHERE id IN (?, ?)",
			taskID,
			rollbackTaskID,
		)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	task, event := newMySQLTaskCreationPair(t, taskID, eventID, now)
	if err := creator.CreateTaskWithEvent(ctx, task, event); err != nil {
		t.Fatalf("CreateTaskWithEvent returned error: %v", err)
	}
	if _, err := taskStore.Get(ctx, taskID); err != nil {
		t.Fatalf("Get committed task returned error: %v", err)
	}
	events, err := eventStore.ListByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("ListByTaskID returned error: %v", err)
	}
	if len(events) != 1 || events[0].ID != eventID {
		t.Fatalf("unexpected committed events: %+v", events)
	}

	rollbackTask, rollbackEvent := newMySQLTaskCreationPair(
		t,
		rollbackTaskID,
		eventID,
		now.Add(time.Second),
	)
	if err := creator.CreateTaskWithEvent(ctx, rollbackTask, rollbackEvent); !errors.Is(err, storeerrors.ErrEventAlreadyExists) {
		t.Fatalf("expected ErrEventAlreadyExists, got %v", err)
	}
	if _, err := taskStore.Get(ctx, rollbackTaskID); !errors.Is(err, storeerrors.ErrTaskNotFound) {
		t.Fatalf("task insert was not rolled back: %v", err)
	}
}

func newMySQLTaskCreationPair(
	t *testing.T,
	taskID string,
	eventID string,
	now time.Time,
) (*domain.Task, *domain.TaskEvent) {
	t.Helper()
	task, err := domain.NewTask(
		taskID,
		"url_check",
		json.RawMessage(`{"urls":["https://example.com"]}`),
		3,
		now,
	)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}
	event, err := domain.NewTaskEvent(
		eventID,
		task.ID,
		domain.EventTaskCreated,
		"task created",
		nil,
		0,
		now,
	)
	if err != nil {
		t.Fatalf("NewTaskEvent returned error: %v", err)
	}
	return task, event
}
