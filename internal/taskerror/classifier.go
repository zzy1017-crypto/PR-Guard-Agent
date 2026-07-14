package taskerror

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func Classify(err error) Classification {
	switch {
	case errors.Is(err, ErrEmbeddingTimeout):
		return retryable(CodeEmbeddingTimeout, "embedding service timed out")
	case errors.Is(err, ErrEmbeddingProviderError):
		return retryable(CodeEmbeddingProviderError, "embedding service is temporarily unavailable")
	case errors.Is(err, ErrQdrantTimeout):
		return retryable(CodeQdrantTimeout, "code retrieval service timed out")
	case errors.Is(err, ErrQdrantUnavailable):
		return retryable(CodeQdrantUnavailable, "code retrieval service is temporarily unavailable")
	case errors.Is(err, context.DeadlineExceeded):
		return retryable(CodeTaskTimeout, "analysis task timed out")
	case errors.Is(err, ErrWorkerStale):
		return retryable(CodeWorkerStale, "analysis worker execution became stale")
	case errors.Is(err, ErrProjectNotFound):
		return permanent(CodeProjectNotFound, "analysis project does not exist")
	case errors.Is(err, ErrDiffNotFound):
		return permanent(CodeDiffNotFound, "analysis diff does not exist")
	case errors.Is(err, ErrDiffProjectMismatch):
		return permanent(CodeDiffProjectMismatch, "analysis diff does not belong to project")
	case errors.Is(err, ErrEmptyDiff):
		return permanent(CodeEmptyDiff, "analysis diff is empty")
	case errors.Is(err, ErrInvalidTopK):
		return permanent(CodeInvalidTopK, "analysis top_k is invalid")
	case errors.Is(err, ErrInvalidTaskData):
		return permanent(CodeInvalidTaskData, "analysis task data is invalid")
	case errors.Is(err, ErrInvalidTaskState):
		return permanent(CodeInvalidTaskState, "analysis task state is invalid")
	}

	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1213:
			return retryable(CodeDatabaseDeadlock, "database operation was deadlocked")
		case 1205:
			return retryable(CodeDatabaseTimeout, "database operation timed out")
		case 1040, 1042, 1043, 1047, 1129, 1130, 2002, 2003, 2006, 2013:
			return retryable(CodeDatabaseUnavailable, "database is temporarily unavailable")
		}
	}

	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return retryable(CodeDatabaseUnavailable, "database is temporarily unavailable")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Temporary() {
		return retryable(CodeDatabaseUnavailable, "database is temporarily unavailable")
	}

	return permanent(CodeInternalAnalysisError, "analysis task failed")
}

func retryable(code, message string) Classification {
	return Classification{Code: code, Retryable: true, PublicMessage: message}
}

func permanent(code, message string) Classification {
	return Classification{Code: code, PublicMessage: message}
}
