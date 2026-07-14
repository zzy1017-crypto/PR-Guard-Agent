package taskerror

import "errors"

const (
	CodeTaskTimeout            = "task_timeout"
	CodeEmbeddingTimeout       = "embedding_timeout"
	CodeEmbeddingProviderError = "embedding_provider_error"
	CodeQdrantTimeout          = "qdrant_timeout"
	CodeQdrantUnavailable      = "qdrant_unavailable"
	CodeDatabaseDeadlock       = "database_deadlock"
	CodeDatabaseTimeout        = "database_timeout"
	CodeDatabaseUnavailable    = "database_unavailable"
	CodeWorkerStale            = "worker_stale"
	CodeProjectNotFound        = "project_not_found"
	CodeDiffNotFound           = "diff_not_found"
	CodeDiffProjectMismatch    = "diff_project_mismatch"
	CodeEmptyDiff              = "empty_diff"
	CodeInvalidTopK            = "invalid_top_k"
	CodeInvalidTaskData        = "invalid_task_data"
	CodeInvalidTaskState       = "invalid_task_state"
	CodeInternalAnalysisError  = "internal_analysis_error"
	CodeRetryExhausted         = "retry_exhausted"
)

var (
	ErrEmbeddingTimeout       = errors.New("embedding timeout")
	ErrEmbeddingProviderError = errors.New("embedding provider temporary error")
	ErrQdrantTimeout          = errors.New("qdrant timeout")
	ErrQdrantUnavailable      = errors.New("qdrant unavailable")
	ErrWorkerStale            = errors.New("analysis worker execution became stale")
	ErrProjectNotFound        = errors.New("project not found")
	ErrDiffNotFound           = errors.New("diff not found")
	ErrDiffProjectMismatch    = errors.New("diff does not belong to project")
	ErrEmptyDiff              = errors.New("diff text is empty")
	ErrInvalidTopK            = errors.New("top_k must be between 1 and 20")
	ErrInvalidTaskData        = errors.New("invalid analysis task data")
	ErrInvalidTaskState       = errors.New("invalid analysis task state")
)

type Classification struct {
	Code          string
	Retryable     bool
	PublicMessage string
}
