package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
)

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type sqlQueryExecutor interface {
	sqlExecutor
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func eventExists(ctx context.Context, executor sqlQueryExecutor, eventID string) (bool, error) {
	var exists int
	err := executor.QueryRowContext(
		ctx,
		"SELECT 1 FROM task_events WHERE id = ? LIMIT 1",
		eventID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
