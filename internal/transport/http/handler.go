package httptransport

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/zhaozhonghe/taskpulse/internal/application"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

const maxRequestBodyBytes = 1 << 20

type Handler struct{ taskService *application.TaskService }

func NewHandler(taskService *application.TaskService) *Handler {
	return &Handler{taskService: taskService}
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
	task, err := h.taskService.CreateTask(r.Context(), application.CreateTaskInput{
		Workflow: request.Workflow, Input: request.Input, MaxRetries: request.MaxRetries,
	})
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	task, err := h.taskService.GetTask(r.Context(), r.PathValue("task_id"))
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
	case errors.Is(err, store.ErrTaskNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrTaskAlreadyExists):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
