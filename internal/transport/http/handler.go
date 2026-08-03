package httptransport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/application"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

const maxRequestBodyBytes = 1 << 20

type Handler struct {
	taskService       *application.TaskService
	workerTaskService *application.WorkerTaskService
}

func NewHandler(taskService *application.TaskService) *Handler {
	return &Handler{taskService: taskService}
}

func NewHandlerWithWorker(
	taskService *application.TaskService,
	workerTaskService *application.WorkerTaskService,
) *Handler {
	return &Handler{
		taskService:       taskService,
		workerTaskService: workerTaskService,
	}
}

type createTaskRequest struct {
	Workflow   string          `json:"workflow"`
	Input      json.RawMessage `json:"input"`
	MaxRetries int             `json:"max_retries"`
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var request createTaskRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.taskService.CreateTask(r.Context(), application.CreateTaskInput{
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Workflow:       request.Workflow,
		Input:          request.Input,
		MaxRetries:     request.MaxRetries,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result.Task)
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	task, err := h.taskService.GetTask(r.Context(), r.PathValue("task_id"))
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) CancelTask(w http.ResponseWriter, r *http.Request) {
	task, err := h.taskService.CancelTask(r.Context(), r.PathValue("task_id"))
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) ListTaskEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.taskService.ListTaskEvents(r.Context(), r.PathValue("task_id"))
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

type workerClaimRequest struct {
	WorkerID      string `json:"worker_id"`
	LeaseDuration string `json:"lease_duration"`
}

type workerHeartbeatRequest struct {
	WorkerID      string `json:"worker_id"`
	LeaseDuration string `json:"lease_duration"`
}

type workerCompleteRequest struct {
	WorkerID string          `json:"worker_id"`
	Version  uint64          `json:"version"`
	Output   json.RawMessage `json:"output"`
}

type workerFailRequest struct {
	WorkerID     string `json:"worker_id"`
	Version      uint64 `json:"version"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func (h *Handler) ClaimWorkerTask(w http.ResponseWriter, r *http.Request) {
	if h.workerTaskService == nil {
		writeError(w, http.StatusNotFound, "worker protocol is not configured")
		return
	}
	var request workerClaimRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	leaseDuration, err := parseLeaseDuration(request.LeaseDuration)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	task, err := h.workerTaskService.ClaimTask(r.Context(), application.ClaimTaskInput{
		WorkerID:      request.WorkerID,
		LeaseDuration: leaseDuration,
	})
	if errors.Is(err, store.ErrNoTaskAvailable) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) HeartbeatWorkerTask(w http.ResponseWriter, r *http.Request) {
	if h.workerTaskService == nil {
		writeError(w, http.StatusNotFound, "worker protocol is not configured")
		return
	}
	var request workerHeartbeatRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	leaseDuration, err := parseLeaseDuration(request.LeaseDuration)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	task, err := h.workerTaskService.HeartbeatTask(r.Context(), application.HeartbeatTaskInput{
		TaskID:        r.PathValue("task_id"),
		WorkerID:      request.WorkerID,
		LeaseDuration: leaseDuration,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) CompleteWorkerTask(w http.ResponseWriter, r *http.Request) {
	if h.workerTaskService == nil {
		writeError(w, http.StatusNotFound, "worker protocol is not configured")
		return
	}
	var request workerCompleteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	task, err := h.workerTaskService.CompleteTask(r.Context(), application.CompleteTaskInput{
		TaskID:  r.PathValue("task_id"),
		WorkerID: request.WorkerID,
		Version: request.Version,
		Output:  request.Output,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) FailWorkerTask(w http.ResponseWriter, r *http.Request) {
	if h.workerTaskService == nil {
		writeError(w, http.StatusNotFound, "worker protocol is not configured")
		return
	}
	var request workerFailRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	task, err := h.workerTaskService.FailTask(r.Context(), application.FailTaskInput{
		TaskID:       r.PathValue("task_id"),
		WorkerID:     request.WorkerID,
		Version:      request.Version,
		ErrorCode:    request.ErrorCode,
		ErrorMessage: request.ErrorMessage,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func parseLeaseDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, fmt.Errorf("%w: lease_duration is required", application.ErrInvalidWorkerRequest)
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%w: lease_duration must be a positive duration", application.ErrInvalidWorkerRequest)
	}
	return duration, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

func writeApplicationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, application.ErrInvalidWorkerRequest):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrTaskNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrTaskAlreadyExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrTaskNotCancelable):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrLeaseLost),
		errors.Is(err, store.ErrTaskConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
