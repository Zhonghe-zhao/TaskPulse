package httptransport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhaozhonghe/taskpulse/internal/application"
	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

func TestCreateGetAndListTaskEvents(t *testing.T) {
	router := newTestRouter()
	body := `{"workflow":"url_check","input":{"urls":["https://example.com"]},"max_retries":3}`
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body)))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}

	var created domain.Task
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode created task: %v", err)
	}
	if created.ID == "" || created.Status != domain.TaskStatusQueued {
		t.Fatalf("unexpected created task: %+v", created)
	}

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/tasks/"+created.ID, nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, getResponse.Code, getResponse.Body.String())
	}

	eventsResponse := httptest.NewRecorder()
	router.ServeHTTP(eventsResponse, httptest.NewRequest(http.MethodGet, "/tasks/"+created.ID+"/events", nil))
	if eventsResponse.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, eventsResponse.Code, eventsResponse.Body.String())
	}

	var events []*domain.TaskEvent
	if err := json.NewDecoder(eventsResponse.Body).Decode(&events); err != nil {
		t.Fatalf("decode task events: %v", err)
	}
	if len(events) != 1 || events[0].Type != domain.EventTaskCreated {
		t.Fatalf("unexpected task events: %+v", events)
	}
}

func TestCreateTaskRejectsInvalidJSON(t *testing.T) {
	router := newTestRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"workflow":`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestGetTaskReturnsNotFound(t *testing.T) {
	router := newTestRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasks/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}

func newTestRouter() http.Handler {
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	taskCreationStore := store.NewMemoryTaskCreationStore(taskStore, eventStore)
	service := application.NewTaskService(taskStore, eventStore, taskCreationStore)
	return NewRouter(NewHandler(service))
}
