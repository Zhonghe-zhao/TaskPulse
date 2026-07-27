package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/identity"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

type ExecutionOutcome string

const (
	OutcomeSucceeded ExecutionOutcome = "succeeded"
	OutcomePartial   ExecutionOutcome = "partially_succeeded"
	OutcomeFailed    ExecutionOutcome = "failed"
)

type ExecutionResult struct {
	Output       json.RawMessage
	Outcome      ExecutionOutcome
	ErrorMessage string
}

type Executor interface {
	Execute(ctx context.Context, task *domain.Task) (ExecutionResult, error)
}

type Worker struct {
	taskStore     store.TaskStore
	eventStore    store.EventStore
	executors     map[string]Executor
	id            string
	leaseDuration time.Duration
	now           func() time.Time
}

const defaultLeaseDuration = 30 * time.Second

func New(taskStore store.TaskStore, eventStore store.EventStore, executors map[string]Executor) *Worker {
	copiedExecutors := make(map[string]Executor, len(executors))
	for workflow, executor := range executors {
		copiedExecutors[workflow] = executor
	}

	return &Worker{
		taskStore:     taskStore,
		eventStore:    eventStore,
		executors:     copiedExecutors,
		id:            identity.New("worker"),
		leaseDuration: defaultLeaseDuration,
		now:           time.Now,
	}
}

// 当前逻辑Worker从队列中领取任务 添加事件 识别工作流 执行相应任务
func (w *Worker) ProcessNext(ctx context.Context) (bool, error) {
	now := w.now()
	task, err := w.taskStore.ClaimNext(ctx, store.ClaimOptions{
		WorkerID:      w.id,
		Now:           now,
		LeaseDuration: w.leaseDuration,
	})
	if errors.Is(err, store.ErrNoTaskAvailable) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim task: %w", err)
	}

	startedEventType := domain.EventTaskStarted
	startedMessage := "task started"
	if task.RetryCount > 0 {
		startedEventType = domain.EventTaskRecovered
		startedMessage = "task recovered after lease expiration"
	}
	if err := w.appendEvent(ctx, task, startedEventType, startedMessage); err != nil {
		return true, fmt.Errorf("append task started event: %w", err)
	}

	executor, exists := w.executors[task.Workflow]
	if !exists {
		err := fmt.Errorf("no executor registered for workflow %q", task.Workflow)
		return true, w.finishFailed(ctx, task, err)
	}

	result, executeErr := w.executeWithHeartbeat(ctx, task, executor)
	if executeErr != nil {
		if errors.Is(executeErr, store.ErrLeaseLost) {
			return true, executeErr
		}
		return true, w.finishFailed(ctx, task, executeErr)
	}
	return true, w.finishWithResult(ctx, task, result)
}

func (w *Worker) executeWithHeartbeat( // 主流程：执行 Executor 后台流程：定期为任务续租
	ctx context.Context,
	task *domain.Task,
	executor Executor,
) (ExecutionResult, error) {
	executionCtx, cancel := context.WithCancel(ctx) //这里从 Worker 的上层 Context 派生出一个子 Context：心跳续租失败时，只取消当前任务的 Executor。
	defer cancel()

	heartbeatResult := make(chan error, 1)
	go w.maintainLease(executionCtx, cancel, task.ID, heartbeatResult) //启动心跳

	result, executeErr := executor.Execute(executionCtx, task)
	cancel()
	heartbeatErr := <-heartbeatResult
	if heartbeatErr != nil {
		return ExecutionResult{}, heartbeatErr
	}
	return result, executeErr
}

func (w *Worker) maintainLease( // 每隔一段时间调用 TaskStore.RenewLease
	ctx context.Context,
	cancelExecution context.CancelFunc,
	taskID string,
	result chan<- error,
) {
	interval := w.leaseDuration / 3
	if interval <= 0 {
		interval = w.leaseDuration
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result <- nil
			return
		case now := <-ticker.C:
			err := w.taskStore.RenewLease(ctx, store.RenewLeaseOptions{
				TaskID:        taskID,
				WorkerID:      w.id,
				Now:           now,
				LeaseDuration: w.leaseDuration,
			})
			if err != nil {
				cancelExecution()
				result <- fmt.Errorf("renew task %q lease: %w", taskID, err)
				return
			}
		}
	}
}

func (w *Worker) Run(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}

	for {
		processed, err := w.ProcessNext(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			if errors.Is(err, store.ErrLeaseLost) {
				continue
			}
			return err
		}
		if processed {
			continue
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (w *Worker) finishWithResult(ctx context.Context, task *domain.Task, result ExecutionResult) error { // 根据 ExecutionResult 更新任务状态
	task.Result = result.Output
	task.Progress = 100
	task.ErrorMessage = result.ErrorMessage

	var status domain.TaskStatus
	var eventType domain.EventType
	var message string
	switch result.Outcome {
	case OutcomeSucceeded:
		status, eventType, message = domain.TaskStatusSucceeded, domain.EventTaskSucceeded, "task succeeded"
	case OutcomePartial:
		status, eventType, message = domain.TaskStatusPartial, domain.EventTaskPartial, "task partially succeeded"
	case OutcomeFailed:
		status, eventType, message = domain.TaskStatusFailed, domain.EventTaskFailed, "task failed"
		if task.ErrorMessage == "" {
			task.ErrorMessage = "executor reported failure"
		}
	default:
		return w.finishFailed(ctx, task, fmt.Errorf("invalid execution outcome %q", result.Outcome))
	}

	if err := task.MoveTo(status, w.now()); err != nil {
		return fmt.Errorf("move task to %s: %w", status, err)
	}
	if err := w.taskStore.Update(ctx, task); err != nil {
		return fmt.Errorf("save %s task: %w", status, err)
	}
	if err := w.appendEvent(ctx, task, eventType, message); err != nil {
		return fmt.Errorf("append %s event: %w", status, err)
	}
	return nil
}

func (w *Worker) finishFailed(ctx context.Context, task *domain.Task, cause error) error {
	task.ErrorMessage = cause.Error()
	if err := task.MoveTo(domain.TaskStatusFailed, w.now()); err != nil {
		return fmt.Errorf("mark task failed: %w", err)
	}
	if err := w.taskStore.Update(ctx, task); err != nil {
		return fmt.Errorf("save failed task: %w", err)
	}
	if err := w.appendEvent(ctx, task, domain.EventTaskFailed, "task failed"); err != nil {
		return fmt.Errorf("append task failed event: %w", err)
	}
	return nil
}

func (w *Worker) appendEvent(ctx context.Context, task *domain.Task, eventType domain.EventType, message string) error {
	event, err := domain.NewTaskEvent(
		identity.New("event"), task.ID, eventType, message,
		json.RawMessage("{}"), task.Progress, w.now(),
	)
	if err != nil {
		return err
	}
	return w.eventStore.Append(ctx, event)
}
