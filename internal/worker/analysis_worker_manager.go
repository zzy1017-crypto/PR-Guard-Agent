package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/service"
	"pr-guard-agent/internal/taskerror"
	"pr-guard-agent/pkg/requestid"
)

type ReportAnalyzer interface {
	AnalyzeDiff(ctx context.Context, projectID uint, diffID uint, topK int) (*service.AnalyzeResult, error)
}

type AnalysisTaskRepository interface {
	ClaimNextPending(ctx context.Context, workerID string) (*model.AnalysisTask, error)
	MarkSucceeded(ctx context.Context, id uint64, workerID string, expectedAttempt int, resultJSON string, reportID *uint, degraded bool) error
	RequeueWithBackoff(ctx context.Context, taskID uint64, workerID string, expectedAttempt int, nextRunAt time.Time, errorCode string, errorMessage string) error
	MarkFailedFinal(ctx context.Context, taskID uint64, workerID string, expectedAttempt int, errorCode string, errorMessage string) error
	RecoverStaleTasks(ctx context.Context, cutoff time.Time, policy taskerror.RetryPolicy) (repository.StaleRecoveryResult, error)
}

type AnalysisWorkerManager struct {
	repository AnalysisTaskRepository
	analyzer   ReportAnalyzer
	config     config.AnalysisWorkerConfig
	logger     *zap.Logger
	registry   *WorkerRuntimeRegistry

	mu          sync.Mutex
	started     bool
	stopOnce    sync.Once
	stopCh      chan struct{}
	doneCh      chan struct{}
	claimCtx    context.Context
	cancelClaim context.CancelFunc
	wg          sync.WaitGroup
}

func NewAnalysisWorkerManager(
	repository AnalysisTaskRepository,
	analyzer ReportAnalyzer,
	cfg config.AnalysisWorkerConfig,
	logger *zap.Logger,
	registries ...*WorkerRuntimeRegistry,
) *AnalysisWorkerManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	registry := NewWorkerRuntimeRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	claimCtx, cancelClaim := context.WithCancel(context.Background())
	return &AnalysisWorkerManager{
		repository:  repository,
		analyzer:    analyzer,
		config:      cfg,
		logger:      logger,
		registry:    registry,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		claimCtx:    claimCtx,
		cancelClaim: cancelClaim,
	}
}

func (m *AnalysisWorkerManager) Start(ctx context.Context) error {
	if !m.config.Enabled {
		return nil
	}
	if m.repository == nil || m.analyzer == nil {
		return errors.New("analysis worker manager dependencies are not initialized")
	}
	if err := m.config.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return nil
	}

	cutoff := time.Now().Add(-time.Duration(m.config.StaleAfterSeconds) * time.Second)
	recovered, err := m.repository.RecoverStaleTasks(ctx, cutoff, m.retryPolicy())
	if err != nil {
		return fmt.Errorf("recover stale analysis tasks failed: %w", err)
	}
	m.logger.Info("stale_tasks_recovered",
		zap.Int64("recovered_count", recovered.Total()),
		zap.Int64("pending_count", recovered.Pending),
		zap.Int64("failed_count", recovered.Failed),
	)
	for _, item := range recovered.Scheduled {
		m.logger.Warn("stale_task_retry_scheduled",
			zap.Uint64("task_id", item.TaskID),
			zap.String("worker_id", item.WorkerID),
			zap.Int("attempt_count", item.AttemptCount),
			zap.Int("max_attempts", item.MaxAttempts),
			zap.Timep("next_run_at", item.NextRunAt),
		)
	}
	for _, item := range recovered.Exhausted {
		m.logger.Warn("stale_task_retry_exhausted",
			zap.Uint64("task_id", item.TaskID),
			zap.Int("attempt_count", item.AttemptCount),
			zap.Int("max_attempts", item.MaxAttempts),
		)
	}

	m.started = true
	for index := 1; index <= m.config.WorkerCount; index++ {
		workerID := buildWorkerID(index)
		m.wg.Add(1)
		go m.runWorker(workerID)
	}
	go func() {
		m.wg.Wait()
		close(m.doneCh)
	}()
	return nil
}

func (m *AnalysisWorkerManager) Shutdown(ctx context.Context) error {
	if !m.config.Enabled {
		return nil
	}
	m.logger.Info("worker_manager_stopping")
	m.stopOnce.Do(func() {
		close(m.stopCh)
		m.cancelClaim()
	})

	select {
	case <-m.doneCh:
		m.logger.Info("worker_manager_stopped")
		return nil
	case <-ctx.Done():
		m.logger.Warn("worker_manager_stop_timeout")
		return ctx.Err()
	}
}

func (m *AnalysisWorkerManager) runWorker(workerID string) {
	defer m.wg.Done()
	m.registry.RegisterWorker(workerID)
	defer m.registry.MarkIdle(workerID)
	m.logger.Info("worker_started", zap.String("worker_id", workerID))
	defer m.logger.Info("worker_stopped", zap.String("worker_id", workerID))

	for {
		if m.stopping() {
			return
		}
		m.registry.MarkPoll(workerID)
		task, err := m.repository.ClaimNextPending(m.claimCtx, workerID)
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrNoPendingTask):
				if !m.waitForNextPoll() {
					return
				}
			case errors.Is(err, context.Canceled) && m.stopping():
				return
			default:
				m.logger.Error("analysis_task_claim_failed", zap.String("worker_id", workerID), zap.Error(err))
				if !m.waitForNextPoll() {
					return
				}
			}
			continue
		}

		fields := workerTaskLogFields(task, workerID)
		m.logger.Info("analysis_task_claimed", fields...)
		taskID := uint64(0)
		if task != nil {
			taskID = task.ID
		}
		m.registry.MarkBusy(workerID, taskID)
		if m.processTask(task, workerID) == attemptSucceeded {
			m.registry.MarkSuccess(workerID)
		} else {
			m.registry.MarkFailure(workerID)
		}
		m.registry.MarkIdle(workerID)
	}
}

type attemptOutcome uint8

const (
	attemptFailed attemptOutcome = iota
	attemptSucceeded
)

func (m *AnalysisWorkerManager) processTask(task *model.AnalysisTask, workerID string) attemptOutcome {
	if task == nil {
		m.logger.Error("analysis_task_invalid_data", zap.String("worker_id", workerID), zap.String("error_code", taskerror.CodeInvalidTaskData))
		return attemptFailed
	}
	fields := workerTaskLogFields(task, workerID)
	m.logger.Info("analysis_task_started", fields...)
	if classification, invalid := classifyTaskData(task, workerID); invalid {
		m.handleFailure(task, workerID, classification)
		return attemptFailed
	}

	taskCtx, cancel := context.WithTimeout(context.Background(), time.Duration(m.config.TaskTimeoutSeconds)*time.Second)
	taskCtx = requestid.WithContext(taskCtx, fmt.Sprintf("analysis-task-%d", task.ID))
	result, err := m.analyzer.AnalyzeDiff(taskCtx, task.ProjectID, task.DiffID, task.TopK)
	timedOut := errors.Is(taskCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil {
		classification := taskerror.Classify(err)
		if timedOut && classification.Code == taskerror.CodeInternalAnalysisError {
			classification = taskerror.Classify(context.DeadlineExceeded)
		}
		m.handleFailure(task, workerID, classification)
		return attemptFailed
	}
	if result == nil {
		m.handleFailure(task, workerID, taskerror.Classify(taskerror.ErrInvalidTaskData))
		return attemptFailed
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		m.handleFailure(task, workerID, taskerror.Classify(taskerror.ErrInvalidTaskData))
		return attemptFailed
	}
	var reportID *uint
	if result.ReportID > 0 {
		id := result.ReportID
		reportID = &id
	}

	persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = m.repository.MarkSucceeded(persistCtx, task.ID, workerID, task.AttemptCount, string(resultJSON), reportID, result.Degraded)
	persistCancel()
	if err != nil {
		m.logger.Error("analysis_task_success_update_failed", append(fields, zap.Error(err))...)
		m.logStateConflict(err, task, workerID)
		return attemptFailed
	}
	m.logger.Info("analysis_task_succeeded", append(fields,
		zap.Bool("degraded", result.Degraded),
		zap.Uint("report_id", result.ReportID),
	)...)
	return attemptSucceeded
}

func (m *AnalysisWorkerManager) handleFailure(task *model.AnalysisTask, workerID string, classification taskerror.Classification) {
	code := sanitizeErrorCode(classification.Code)
	message := sanitizeErrorMessage(classification.PublicMessage)
	if classification.Retryable && task.AttemptCount < task.MaxAttempts {
		delay := m.retryPolicy().NextDelay(task.AttemptCount)
		nextRunAt := time.Now().Add(delay)
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := m.repository.RequeueWithBackoff(persistCtx, task.ID, workerID, task.AttemptCount, nextRunAt, code, message)
		cancel()
		fields := workerTaskLogFields(task, workerID)
		if err != nil {
			m.logger.Error("analysis_task_retry_update_failed", append(fields, zap.Error(err))...)
			m.logStateConflict(err, task, workerID)
			return
		}
		m.logger.Warn("analysis_task_retry_scheduled", append(fields,
			zap.Int("attempt_count", task.AttemptCount),
			zap.Int("max_attempts", task.MaxAttempts),
			zap.String("error_code", code),
			zap.Int64("retry_delay_ms", delay.Milliseconds()),
			zap.Time("next_run_at", nextRunAt),
		)...)
		return
	}

	logEvent := "analysis_task_permanent_failure"
	if classification.Retryable {
		logEvent = "analysis_task_retry_exhausted"
		code = taskerror.CodeRetryExhausted
		message = sanitizeErrorMessage("analysis failed after maximum attempts: " + classification.PublicMessage)
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := m.repository.MarkFailedFinal(persistCtx, task.ID, workerID, task.AttemptCount, code, message)
	cancel()
	fields := workerTaskLogFields(task, workerID)
	if err != nil {
		m.logger.Error("analysis_task_failure_update_failed", append(fields, zap.Error(err))...)
		m.logStateConflict(err, task, workerID)
		return
	}
	if classification.Retryable {
		m.logger.Warn(logEvent, append(fields,
			zap.Int("attempt_count", task.AttemptCount),
			zap.String("last_error_code", classification.Code),
		)...)
		return
	}
	m.logger.Warn(logEvent, append(fields,
		zap.Int("attempt_count", task.AttemptCount),
		zap.String("error_code", code),
	)...)
}

func (m *AnalysisWorkerManager) waitForNextPoll() bool {
	timer := time.NewTimer(time.Duration(m.config.PollIntervalMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-m.stopCh:
		return false
	}
}

func (m *AnalysisWorkerManager) stopping() bool {
	select {
	case <-m.stopCh:
		return true
	default:
		return false
	}
}

func (m *AnalysisWorkerManager) RuntimeRegistry() *WorkerRuntimeRegistry {
	if m == nil {
		return nil
	}
	return m.registry
}

func (m *AnalysisWorkerManager) Stopping() bool {
	if m == nil {
		return false
	}
	return m.stopping()
}

func sanitizeErrorCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return taskerror.CodeInternalAnalysisError
	}
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func sanitizeErrorMessage(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' || (r >= 0x20 && r != 0x7f) {
			return r
		}
		return -1
	}, strings.TrimSpace(value))
	for len(value) > 512 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	if value == "" {
		return "analysis task failed"
	}
	return value
}

func workerTaskLogFields(task *model.AnalysisTask, workerID string) []zap.Field {
	return []zap.Field{
		zap.Uint64("task_id", task.ID),
		zap.String("worker_id", workerID),
		zap.Uint("project_id", task.ProjectID),
		zap.Uint("diff_id", task.DiffID),
	}
}

func (m *AnalysisWorkerManager) retryPolicy() taskerror.RetryPolicy {
	return taskerror.RetryPolicy{
		BaseDelay:     time.Duration(m.config.RetryBaseSeconds) * time.Second,
		MaxDelay:      time.Duration(m.config.RetryMaxSeconds) * time.Second,
		JitterPercent: m.config.RetryJitterPercent,
	}
}

func (m *AnalysisWorkerManager) logStateConflict(err error, task *model.AnalysisTask, workerID string) {
	if !errors.Is(err, repository.ErrTaskStateConflict) {
		return
	}
	m.logger.Warn("analysis_task_state_conflict",
		zap.Uint64("task_id", task.ID),
		zap.String("worker_id", workerID),
		zap.Int("expected_attempt", task.AttemptCount),
	)
}

func classifyTaskData(task *model.AnalysisTask, workerID string) (taskerror.Classification, bool) {
	if task == nil || task.ID == 0 || task.ProjectID == 0 || task.DiffID == 0 || task.MaxAttempts < 1 || task.AttemptCount < 1 {
		return taskerror.Classify(taskerror.ErrInvalidTaskData), true
	}
	if task.TopK < 1 || task.TopK > 20 {
		return taskerror.Classify(taskerror.ErrInvalidTopK), true
	}
	if task.Status != model.AnalysisTaskStatusRunning || task.WorkerID != workerID {
		return taskerror.Classify(taskerror.ErrInvalidTaskState), true
	}
	return taskerror.Classification{}, false
}

func buildWorkerID(index int) string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "pr-guard"
	}
	random := strings.ReplaceAll(requestid.New(), "-", "")
	return fmt.Sprintf("%s-%d-%d-%s", hostname, os.Getpid(), index, random[:8])
}
