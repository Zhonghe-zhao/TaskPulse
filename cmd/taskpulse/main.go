package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/application"
	"github.com/zhaozhonghe/taskpulse/internal/executor/urlcheck"
	"github.com/zhaozhonghe/taskpulse/internal/store"
	httptransport "github.com/zhaozhonghe/taskpulse/internal/transport/http"
	"github.com/zhaozhonghe/taskpulse/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	taskCreationStore := store.NewMemoryTaskCreationStore(taskStore, eventStore)
	taskService := application.NewTaskService(taskStore, eventStore, taskCreationStore)
	router := httptransport.NewRouter(httptransport.NewHandler(taskService))

	urlCheckExecutor := urlcheck.NewWithConcurrency(
		&http.Client{Timeout: 10 * time.Second},
		5,
	)
	taskWorker := worker.New(taskStore, eventStore, map[string]worker.Executor{
		"url_check": urlCheckExecutor,
	})
	taskReaper := worker.NewReaper(taskStore, eventStore)

	backgroundErrors := make(chan error, 2)
	go func() {
		backgroundErrors <- taskWorker.Run(ctx, 200*time.Millisecond)
	}()
	go func() {
		backgroundErrors <- taskReaper.Run(ctx, time.Second)
	}()

	server := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	log.Printf("TaskPulse HTTP server listening on %s", server.Addr)
	select {
	case <-ctx.Done():
		log.Print("shutdown signal received")
	case err := <-backgroundErrors:
		if err != nil {
			log.Printf("background processor stopped: %v", err)
		}
		stop()
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server stopped: %v", err)
		}
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown: %v", err)
	}
}
