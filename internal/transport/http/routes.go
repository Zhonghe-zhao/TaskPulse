package httptransport

import "net/http"

func NewRouter(handler *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", handler.CreateTask)
	mux.HandleFunc("GET /tasks/{task_id}", handler.GetTask)
	mux.HandleFunc("POST /tasks/{task_id}/cancel", handler.CancelTask)
	mux.HandleFunc("GET /tasks/{task_id}/events", handler.ListTaskEvents)
	return mux
}
