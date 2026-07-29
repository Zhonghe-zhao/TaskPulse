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
	httptransport "github.com/zhaozhonghe/taskpulse/internal/transport/http"
	"github.com/zhaozhonghe/taskpulse/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	backend := normalizeStorageBackend(os.Getenv("TASKPULSE_STORAGE"))
	stores, err := openRuntimeStores(ctx, backend)
	if err != nil {
		log.Fatalf("initialize %s storage: %v", backend, err)
	}
	defer func() {
		if err := stores.close(); err != nil {
			log.Printf("close %s storage: %v", backend, err)
		}
	}()
	log.Printf("TaskPulse storage backend: %s", backend)

	taskService := application.NewTaskService(
		stores.tasks,
		stores.events,
		stores.taskCreation,
		stores.taskTransition,
	)
	router := httptransport.NewRouter(httptransport.NewHandler(taskService))

	urlCheckExecutor := urlcheck.NewWithConcurrency(
		&http.Client{Timeout: 10 * time.Second},
		5,
	)
	taskWorker := worker.New(stores.tasks, stores.taskTransition, map[string]worker.Executor{
		"url_check": urlCheckExecutor,
	}, nil)
	taskReaper := worker.NewReaper(stores.taskTransition)

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
