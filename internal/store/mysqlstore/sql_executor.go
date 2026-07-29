package mysqlstore

import (
	"context"
	"database/sql"
)

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type sqlQueryExecutor interface {
	sqlExecutor
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
