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

func TestCreateTaskSupportsIdempotentReplayAndConflict(t *testing.T) {
	router := newTestRouter()
	firstRequest := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(`{"workflow":"llm_analysis","input":{"subject":"go"},"max_retries":3}`),
	)
	firstRequest.Header.Set("Idempotency-Key", "memobridge-analysis-1")
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("expected first status %d, got %d: %s", http.StatusCreated, firstResponse.Code, firstResponse.Body.String())
	}
	var first domain.Task
	if err := json.NewDecoder(firstResponse.Body).Decode(&first); err != nil {
		t.Fatalf("decode first task: %v", err)
	}

	replayRequest := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(`{"workflow":"llm_analysis","input":{"subject":"go"},"max_retries":3}`),
	)
	replayRequest.Header.Set("Idempotency-Key", "memobridge-analysis-1")
	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusOK {
		t.Fatalf("expected replay status %d, got %d: %s", http.StatusOK, replayResponse.Code, replayResponse.Body.String())
	}
	var replayed domain.Task
	if err := json.NewDecoder(replayResponse.Body).Decode(&replayed); err != nil {
		t.Fatalf("decode replayed task: %v", err)
	}
	if replayed.ID != first.ID {
		t.Fatalf("expected replayed task %s, got %s", first.ID, replayed.ID)
	}

	conflictRequest := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(`{"workflow":"llm_analysis","input":{"subject":"database"},"max_retries":3}`),
	)
	conflictRequest.Header.Set("Idempotency-Key", "memobridge-analysis-1")
	conflictResponse := httptest.NewRecorder()
	router.ServeHTTP(conflictResponse, conflictRequest)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("expected conflict status %d, got %d: %s", http.StatusConflict, conflictResponse.Code, conflictResponse.Body.String())
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

func TestCancelTaskIsIdempotent(t *testing.T) {
	router := newTestRouter()
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(
		createResponse,
		httptest.NewRequest(
			http.MethodPost,
			"/tasks",
			strings.NewReader(`{"workflow":"llm_analysis","input":{"subject":"go"}}`),
		),
	)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}
	var created domain.Task
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode created task: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		response := httptest.NewRecorder()
		router.ServeHTTP(
			response,
			httptest.NewRequest(http.MethodPost, "/tasks/"+created.ID+"/cancel", nil),
		)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected status %d, got %d: %s", attempt, http.StatusOK, response.Code, response.Body.String())
		}
		var canceled domain.Task
		if err := json.NewDecoder(response.Body).Decode(&canceled); err != nil {
			t.Fatalf("attempt %d: decode canceled task: %v", attempt, err)
		}
		if canceled.Status != domain.TaskStatusCanceled {
			t.Fatalf("attempt %d: unexpected canceled task: %+v", attempt, canceled)
		}
	}

	eventsResponse := httptest.NewRecorder()
	router.ServeHTTP(
		eventsResponse,
		httptest.NewRequest(http.MethodGet, "/tasks/"+created.ID+"/events", nil),
	)
	var events []*domain.TaskEvent
	if err := json.NewDecoder(eventsResponse.Body).Decode(&events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 2 || events[1].Type != domain.EventTaskCanceled {
		t.Fatalf("unexpected events after repeated cancellation: %+v", events)
	}
}

func TestCancelTaskReturnsNotFound(t *testing.T) {
	router := newTestRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/tasks/missing/cancel", nil),
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, response.Code, response.Body.String())
	}
}

func newTestRouter() http.Handler {
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	taskCreationStore := store.NewMemoryTaskCreationStore(taskStore, eventStore)
	taskTransitionStore := store.NewMemoryTaskTransitionStore(taskStore, eventStore)
	service := application.NewTaskService(
		taskStore,
		eventStore,
		taskCreationStore,
		taskTransitionStore,
	)
	return NewRouter(NewHandler(service))
}
