package httptransport

import "net/http"

func NewRouter(handler *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", handler.CreateTask)
	mux.HandleFunc("GET /tasks/{task_id}", handler.GetTask)
	mux.HandleFunc("GET /tasks/{task_id}/events", handler.ListTaskEvents)
	return mux
}
