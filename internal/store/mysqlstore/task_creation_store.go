package mysqlstore

import (
	"context"
	"database/sql"
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
) (err error) {
	if task == nil {
		return storeerrors.ErrNilTask
	}
	if event == nil {
		return storeerrors.ErrNilEvent
	}
	if event.TaskID != task.ID {
		return storeerrors.ErrTaskEventMismatch
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin task creation transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = insertTask(ctx, tx, task); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit task %q creation: %w", task.ID, err)
	}
	return nil
}
