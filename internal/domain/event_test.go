package domain

import (
	"testing"
	"time"
)

func TestNewTaskEvent(t *testing.T) {
	now := time.Date(2026, 6, 11, 15, 0, 0, 0, time.UTC)

	event, err := NewTaskEvent("event_1", "task_1", EventTaskCreated, "task created", nil, 0, now)
	if err != nil {
		t.Fatalf("NewTaskEvent returned error: %v", err)
	}

	if event.TaskID != "task_1" {
		t.Fatalf("expected task id task_1, got %s", event.TaskID)
	}
	if event.Type != EventTaskCreated {
		t.Fatalf("expected task_created event, got %s", event.Type)
	}
}

func TestRejectInvalidProgress(t *testing.T) {
	now := time.Date(2026, 6, 11, 15, 0, 0, 0, time.UTC)

	if _, err := NewTaskEvent("event_1", "task_1", EventTaskCreated, "task created", nil, 101, now); err == nil {
		t.Fatalf("expected progress > 100 to be rejected")
	}
}
