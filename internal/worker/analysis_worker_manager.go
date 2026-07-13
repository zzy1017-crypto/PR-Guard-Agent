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
	"pr-guard-agent/pkg/requestid"
)

type ReportAnalyzer interface {
	AnalyzeDiff(ctx context.Context, projectID uint, diffID uint, topK int) (*service.AnalyzeResult, error)
}

type AnalysisTaskRepository interface {
	ClaimNextPending(ctx context.Context, workerID string) (*model.AnalysisTask, error)
	MarkSucceeded(ctx context.Context, id uint64, resultJSON string, reportID *uint, degraded bool) error
	MarkFailed(ctx context.Context, id uint64, errorCode, errorMessage string) error
	RecoverStaleTasks(ctx context.Context, cutoff time.Time) (repository.StaleRecoveryResult, error)
}

type AnalysisWorkerManager struct {
	repository AnalysisTaskRepository
	analyzer   ReportAnalyzer
	config     config.AnalysisWorkerConfig
	logger     *zap.Logger

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
) *AnalysisWorkerManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	claimCtx, cancelClaim := context.WithCancel(context.Background())
	return &AnalysisWorkerManager{
		repository:  repository,
		analyzer:    analyzer,
		config:      cfg,
		logger:      logger,
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
	recovered, err := m.repository.RecoverStaleTasks(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("recover stale analysis tasks failed: %w", err)
	}
	m.logger.Info("stale_tasks_recovered",
		zap.Int64("recovered_count", recovered.Total()),
		zap.Int64("pending_count", recovered.Pending),
		zap.Int64("failed_count", recovered.Failed),
	)

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
	m.logger.Info("worker_started", zap.String("worker_id", workerID))
	defer m.logger.Info("worker_stopped", zap.String("worker_id", workerID))

	for {
		if m.stopping() {
			return
		}
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
		m.processTask(task, workerID)
	}
}

func (m *AnalysisWorkerManager) processTask(task *model.AnalysisTask, workerID string) {
	fields := workerTaskLogFields(task, workerID)
	m.logger.Info("analysis_task_started", fields...)

	taskCtx, cancel := context.WithTimeout(context.Background(), time.Duration(m.config.TaskTimeoutSeconds)*time.Second)
	taskCtx = requestid.WithContext(taskCtx, fmt.Sprintf("analysis-task-%d", task.ID))
	result, err := m.analyzer.AnalyzeDiff(taskCtx, task.ProjectID, task.DiffID, task.TopK)
	cancel()
	if err != nil {
		code, message := safeAnalysisError(err, taskCtx)
		m.failTask(task, workerID, code, message)
		return
	}
	if result == nil {
		m.failTask(task, workerID, "invalid_analysis_result", "analysis task did not produce a valid result")
		return
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		m.failTask(task, workerID, "result_encode_failed", "analysis task result could not be encoded")
		return
	}
	var reportID *uint
	if result.ReportID > 0 {
		id := result.ReportID
		reportID = &id
	}

	persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = m.repository.MarkSucceeded(persistCtx, task.ID, string(resultJSON), reportID, result.Degraded)
	persistCancel()
	if err != nil {
		m.logger.Error("analysis_task_success_update_failed", append(fields, zap.Error(err))...)
		return
	}
	m.logger.Info("analysis_task_succeeded", append(fields,
		zap.Bool("degraded", result.Degraded),
		zap.Uint("report_id", result.ReportID),
	)...)
}

func (m *AnalysisWorkerManager) failTask(task *model.AnalysisTask, workerID, code, message string) {
	code = sanitizeErrorCode(code)
	message = sanitizeErrorMessage(message)
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := m.repository.MarkFailed(persistCtx, task.ID, code, message)
	cancel()
	fields := workerTaskLogFields(task, workerID)
	if err != nil {
		m.logger.Error("analysis_task_failure_update_failed", append(fields, zap.Error(err))...)
		return
	}
	m.logger.Warn("analysis_task_failed", append(fields, zap.String("error_code", code))...)
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

func safeAnalysisError(err error, taskCtx context.Context) (string, string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
		return "analysis_task_timeout", "analysis task timed out"
	}
	switch {
	case errors.Is(err, service.ErrProjectNotFound):
		return "project_not_found", "analysis project no longer exists"
	case errors.Is(err, service.ErrDiffNotFound):
		return "diff_not_found", "analysis diff no longer exists"
	case errors.Is(err, service.ErrDiffProjectMismatch):
		return "diff_project_mismatch", "analysis diff does not belong to project"
	case strings.Contains(strings.ToLower(err.Error()), "retrieve related chunks"):
		return "qdrant_search_failed", "analysis context retrieval failed"
	default:
		return "analysis_failed", "analysis task failed"
	}
}

func sanitizeErrorCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "analysis_failed"
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

func buildWorkerID(index int) string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "pr-guard"
	}
	random := strings.ReplaceAll(requestid.New(), "-", "")
	return fmt.Sprintf("%s-%d-%d-%s", hostname, os.Getpid(), index, random[:8])
}
