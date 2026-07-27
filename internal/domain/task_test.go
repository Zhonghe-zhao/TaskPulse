package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewTaskDefaultsToQueued(t *testing.T) {
	now := time.Date(2026, 6, 11, 15, 0, 0, 0, time.UTC)

	task, err := NewTask("task_1", "url_check", json.RawMessage(`{"urls":["https://example.com"]}`), 3, now)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}

	if task.Status != TaskStatusQueued {
		t.Fatalf("expected queued status, got %s", task.Status)
	}
	if task.Progress != 0 {
		t.Fatalf("expected progress 0, got %d", task.Progress)
	}
	if task.CreatedAt != now || task.UpdatedAt != now {
		t.Fatalf("expected timestamps to equal now")
	}
}

func TestTaskStatusTransition(t *testing.T) {
	now := time.Date(2026, 6, 11, 15, 0, 0, 0, time.UTC)
	task, err := NewTask("task_1", "url_check", nil, 3, now)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}

	if err := task.MoveTo(TaskStatusRunning, now.Add(time.Second)); err != nil {
		t.Fatalf("MoveTo running returned error: %v", err)
	}
	if task.StartedAt == nil {
		t.Fatalf("expected StartedAt to be set")
	}
	leaseExpiresAt := now.Add(time.Minute)
	task.LeaseOwner = "worker_1"
	task.LeaseExpiresAt = &leaseExpiresAt

	if err := task.MoveTo(TaskStatusSucceeded, now.Add(2*time.Second)); err != nil {
		t.Fatalf("MoveTo succeeded returned error: %v", err)
	}
	if !task.IsTerminal() {
		t.Fatalf("expected task to be terminal")
	}
	if task.FinishedAt == nil {
		t.Fatalf("expected FinishedAt to be set")
	}
	if task.LeaseOwner != "" || task.LeaseExpiresAt != nil {
		t.Fatalf("expected terminal task lease to be cleared")
	}
}

func TestRejectInvalidTransition(t *testing.T) {
	now := time.Date(2026, 6, 11, 15, 0, 0, 0, time.UTC)
	task, err := NewTask("task_1", "url_check", nil, 3, now)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}

	if err := task.MoveTo(TaskStatusSucceeded, now.Add(time.Second)); err == nil {
		t.Fatalf("expected queued -> succeeded to be rejected")
	}
}

func TestRunningTaskCanPartiallySucceed(t *testing.T) {
	now := time.Date(2026, 6, 11, 15, 0, 0, 0, time.UTC)
	task, err := NewTask("task_1", "url_check", nil, 3, now)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}

	if err := task.MoveTo(TaskStatusRunning, now.Add(time.Second)); err != nil {
		t.Fatalf("MoveTo running returned error: %v", err)
	}
	if err := task.MoveTo(TaskStatusPartial, now.Add(2*time.Second)); err != nil {
		t.Fatalf("MoveTo partially_succeeded returned error: %v", err)
	}
	if !task.IsTerminal() {
		t.Fatalf("expected partially succeeded task to be terminal")
	}
}
