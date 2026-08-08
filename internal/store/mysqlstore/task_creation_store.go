package mysqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	storeerrors "github.com/zhaozhonghe/taskpulse/internal/store"
)

type MySQLTaskCreationStore struct {
	db *sql.DB
}

var _ storeerrors.TaskCreationStore = (*MySQLTaskCreationStore)(nil)

func NewTaskCreationStore(db *sql.DB) (*MySQLTaskCreationStore, error) {
	if db == nil {
		return nil, errors.New("mysql task creation store database is nil")
	}
	return &MySQLTaskCreationStore{db: db}, nil
}

func (s *MySQLTaskCreationStore) CreateTaskWithEvent(
	ctx context.Context,
	task *domain.Task,
	event *domain.TaskEvent,
) (*storeerrors.TaskCreationResult, error) {
	if task == nil {
		return nil, storeerrors.ErrNilTask
	}
	if event == nil {
		return nil, storeerrors.ErrNilEvent
	}
	if event.TaskID != task.ID {
		return nil, storeerrors.ErrTaskEventMismatch
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin task creation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertTask(ctx, tx, task); err != nil {
		if errors.Is(err, storeerrors.ErrIdempotencyKeyAlreadyExists) {
			_ = tx.Rollback()
			existing, resolveErr := s.resolveIdempotentReplay(ctx, task)
			if resolveErr != nil {
				return nil, resolveErr
			}
			return &storeerrors.TaskCreationResult{
				Task:    existing,
				Created: false,
			}, nil
		}
		return nil, err
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := insertTaskOutbox(ctx, tx, task); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit task %q creation: %w", task.ID, err)
	}
	return &storeerrors.TaskCreationResult{
		Task:    task,
		Created: true,
	}, nil
}

func insertTaskOutbox(ctx context.Context, tx *sql.Tx, task *domain.Task) error {
	payload, err := json.Marshal(struct {
		TaskID   string `json:"task_id"`
		Workflow string `json:"workflow"`
	}{TaskID: task.ID, Workflow: task.Workflow})
	if err != nil {
		return fmt.Errorf("marshal task %q outbox payload: %w", task.ID, err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO task_outbox (
    id, task_id, workflow, event_type, payload_json, status,
    attempts, available_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID+":created",
		task.ID,
		task.Workflow,
		"task_ready",
		payload,
		"pending",
		0,
		task.AvailableAt,
		task.CreatedAt,
		task.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task %q outbox record: %w", task.ID, err)
	}
	return nil
}

func (s *MySQLTaskCreationStore) resolveIdempotentReplay(
	ctx context.Context,
	requested *domain.Task,
) (*domain.Task, error) {
	taskStore := &MySQLTaskStore{db: s.db}
	existing, err := taskStore.getByWorkflowAndIdempotencyKey(ctx, requested.Workflow, requested.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if !storeerrors.SameTaskCreationRequest(existing, requested) {
		return nil, storeerrors.ErrIdempotencyConflict
	}
	return existing, nil
}
