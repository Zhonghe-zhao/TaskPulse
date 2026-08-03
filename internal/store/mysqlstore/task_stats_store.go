package mysqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	storeerrors "github.com/zhaozhonghe/taskpulse/internal/store"
)

var _ storeerrors.TaskStatsStore = (*MySQLTaskStore)(nil)

func (s *MySQLTaskStore) SnapshotTaskStats(
	ctx context.Context,
	now time.Time,
) (*storeerrors.TaskStatsSnapshot, error) {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	snapshot := storeerrors.NewTaskStatsSnapshot()
	if err := scanStatusCounts(ctx, s.db, snapshot); err != nil {
		return nil, err
	}
	if err := scanAvailableStats(ctx, s.db, snapshot, now); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func scanStatusCounts(
	ctx context.Context,
	db *sql.DB,
	snapshot *storeerrors.TaskStatsSnapshot,
) error {
	rows, err := db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM tasks
		GROUP BY status`)
	if err != nil {
		return fmt.Errorf("select task status counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return fmt.Errorf("scan task status count: %w", err)
		}
		snapshot.StatusCounts[domain.TaskStatus(status)] = count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate task status counts: %w", err)
	}
	return nil
}

func scanAvailableStats(
	ctx context.Context,
	db *sql.DB,
	snapshot *storeerrors.TaskStatsSnapshot,
	now time.Time,
) error {
	rows, err := db.QueryContext(ctx, `
		SELECT status, COUNT(*), MIN(available_at)
		FROM tasks
		WHERE status IN (?, ?)
		  AND available_at <= ?
		GROUP BY status`,
		string(domain.TaskStatusQueued),
		string(domain.TaskStatusRetrying),
		now,
	)
	if err != nil {
		return fmt.Errorf("select available task stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		var oldest time.Time
		if err := rows.Scan(&status, &count, &oldest); err != nil {
			return fmt.Errorf("scan available task stats: %w", err)
		}
		taskStatus := domain.TaskStatus(status)
		snapshot.AvailableCounts[taskStatus] = count
		snapshot.OldestAvailableAge[taskStatus] = now.Sub(oldest.UTC())
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate available task stats: %w", err)
	}
	return nil
}
