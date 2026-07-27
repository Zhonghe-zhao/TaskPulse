package domain

import (
	"encoding/json"
	"errors"
	"time"
)

type EventType string

const (
	EventTaskCreated   EventType = "task_created"
	EventTaskStarted   EventType = "task_started"
	EventTaskRecovered EventType = "task_recovered"
	EventTaskProgress  EventType = "task_progress"
	EventTaskSucceeded EventType = "task_succeeded"
	EventTaskPartial   EventType = "task_partially_succeeded"
	EventTaskFailed    EventType = "task_failed"
	EventTaskCanceled  EventType = "task_canceled"
	EventItemStarted   EventType = "item_started"
	EventItemSucceeded EventType = "item_succeeded"
	EventItemFailed    EventType = "item_failed"
	EventItemRetrying  EventType = "item_retrying"
)

type TaskEvent struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"task_id"`
	Type      EventType       `json:"type"`
	Message   string          `json:"message"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Progress  int             `json:"progress"`
	CreatedAt time.Time       `json:"created_at"`
}

func NewTaskEvent(id, taskID string, eventType EventType, message string, payload json.RawMessage, progress int, now time.Time) (*TaskEvent, error) {
	if id == "" {
		return nil, errors.New("event id is required")
	}
	if taskID == "" {
		return nil, errors.New("task id is required")
	}
	if eventType == "" {
		return nil, errors.New("event type is required")
	}
	if progress < 0 || progress > 100 {
		return nil, errors.New("progress must be between 0 and 100")
	}
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	return &TaskEvent{
		ID:        id,
		TaskID:    taskID,
		Type:      eventType,
		Message:   message,
		Payload:   payload,
		Progress:  progress,
		CreatedAt: now,
	}, nil
}
