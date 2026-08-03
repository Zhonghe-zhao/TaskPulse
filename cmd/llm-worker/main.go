package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/executor/llmanalysis"
	"github.com/zhaozhonghe/taskpulse/internal/worker"
)

type config struct {
	TaskPulseURL string
	WorkerID     string
	Lease        time.Duration
	PollInterval time.Duration
	HTTPClient   *http.Client
}

type workerClient struct {
	baseURL string
	worker  string
	lease   time.Duration
	client  *http.Client
}

type apiError struct {
	status int
	body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("taskpulse returned status %d: %s", e.status, e.body)
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid worker configuration", "error", err)
		os.Exit(1)
	}

	fakeClient := llmanalysis.NewFakeClient("fake-llm")
	if err := fakeClient.SetFailureMode(os.Getenv("TASKPULSE_LLM_FAKE_FAILURE")); err != nil {
		logger.Error("configure fake llm client", "error", err)
		os.Exit(1)
	}
	if rawDelay := os.Getenv("TASKPULSE_LLM_FAKE_DELAY"); rawDelay != "" {
		delay, err := time.ParseDuration(rawDelay)
		if err != nil || delay < 0 {
			logger.Error("configure fake llm delay", "error", err)
			os.Exit(1)
		}
		if err := fakeClient.SetDelay(delay); err != nil {
			logger.Error("configure fake llm delay", "error", err)
			os.Exit(1)
		}
	}
	executor, err := llmanalysis.New(fakeClient)
	if err != nil {
		logger.Error("initialize llm executor", "error", err)
		os.Exit(1)
	}

	client := &workerClient{
		baseURL: cfg.TaskPulseURL,
		worker:  cfg.WorkerID,
		lease:   cfg.Lease,
		client:  cfg.HTTPClient,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("external llm worker started", "worker_id", cfg.WorkerID, "taskpulse_url", cfg.TaskPulseURL)

	for {
		processed, err := processNext(ctx, client, executor, logger)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			logger.Error("external worker iteration failed", "error", err)
		}
		if processed {
			continue
		}
		timer := time.NewTimer(cfg.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func processNext(
	ctx context.Context,
	client *workerClient,
	executor worker.Executor,
	logger *slog.Logger,
) (bool, error) {
	task, err := client.claim(ctx)
	if errors.Is(err, errNoTaskAvailable) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	logger.Info("external task claimed", "task_id", task.ID, "workflow", task.Workflow, "version", task.Version)

	result, executeErr := executeWithHeartbeat(ctx, client, executor, task)
	if executeErr != nil {
		code := "external_worker_error"
		if classified, ok := worker.AsExecutionError(executeErr); ok {
			code = classified.Code
		}
		if err := client.fail(ctx, task, code, executeErr.Error()); err != nil {
			if isConflict(err) {
				logger.Warn("external task result rejected", "task_id", task.ID, "error", err)
				return true, nil
			}
			return true, err
		}
		logger.Warn("external task failed", "task_id", task.ID, "error_code", code, "error", executeErr)
		return true, nil
	}
	if err := client.complete(ctx, task, result.Output); err != nil {
		if isConflict(err) {
			logger.Warn("external task result rejected", "task_id", task.ID, "error", err)
			return true, nil
		}
		return true, err
	}
	logger.Info("external task completed", "task_id", task.ID)
	return true, nil
}

func executeWithHeartbeat(
	ctx context.Context,
	client *workerClient,
	executor worker.Executor,
	task *domain.Task,
) (worker.ExecutionResult, error) {
	executionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatResult := make(chan error, 1)
	go client.heartbeatLoop(executionCtx, cancel, task, heartbeatResult)

	result, executeErr := executor.Execute(executionCtx, task)
	cancel()
	heartbeatErr := <-heartbeatResult
	if heartbeatErr != nil {
		return worker.ExecutionResult{}, heartbeatErr
	}
	return result, executeErr
}

func (c *workerClient) claim(ctx context.Context) (*domain.Task, error) {
	var task domain.Task
	status, err := c.post(ctx, "/worker/tasks/claim", map[string]string{
		"worker_id":      c.worker,
		"lease_duration": c.lease.String(),
	}, &task)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent {
		return nil, errNoTaskAvailable
	}
	return &task, nil
}

func (c *workerClient) heartbeatLoop(
	ctx context.Context,
	cancel context.CancelFunc,
	task *domain.Task,
	result chan<- error,
) {
	ticker := time.NewTicker(c.lease / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			result <- nil
			return
		case <-ticker.C:
			var renewed domain.Task
			_, err := c.post(ctx, "/worker/tasks/"+task.ID+"/heartbeat", map[string]string{
				"worker_id":      c.worker,
				"lease_duration": c.lease.String(),
			}, &renewed)
			if err != nil {
				cancel()
				result <- err
				return
			}
		}
	}
}

func (c *workerClient) complete(ctx context.Context, task *domain.Task, output json.RawMessage) error {
	_, err := c.post(ctx, "/worker/tasks/"+task.ID+"/complete", map[string]any{
		"worker_id": c.worker,
		"version":   task.Version,
		"output":    output,
	}, nil)
	return err
}

func (c *workerClient) fail(ctx context.Context, task *domain.Task, code, message string) error {
	_, err := c.post(ctx, "/worker/tasks/"+task.ID+"/fail", map[string]any{
		"worker_id":     c.worker,
		"version":       task.Version,
		"error_code":    code,
		"error_message": message,
	}, nil)
	return err
}

func (c *workerClient) post(ctx context.Context, path string, body any, response any) (int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, &apiError{status: resp.StatusCode, body: strings.TrimSpace(string(data))}
	}
	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

var errNoTaskAvailable = errors.New("no task available")

func isConflict(err error) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.status == http.StatusConflict
}

func loadConfig() (config, error) {
	lease, err := parseDurationEnv("TASKPULSE_EXTERNAL_LEASE", 30*time.Second)
	if err != nil {
		return config{}, err
	}
	poll, err := parseDurationEnv("TASKPULSE_EXTERNAL_POLL_INTERVAL", 200*time.Millisecond)
	if err != nil {
		return config{}, err
	}
	workerID := os.Getenv("TASKPULSE_EXTERNAL_WORKER_ID")
	if workerID == "" {
		workerID = "external-llm-worker"
	}
	return config{
		TaskPulseURL: strings.TrimRight(defaultEnv("TASKPULSE_URL", "http://localhost:8080"), "/"),
		WorkerID:     workerID,
		Lease:        lease,
		PollInterval: poll,
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func parseDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func defaultEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
