package taskerror

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyContextDeadlineRetryable(t *testing.T) {
	got := Classify(context.DeadlineExceeded)
	if got.Code != CodeTaskTimeout || !got.Retryable {
		t.Fatalf("Classify() = %#v", got)
	}
}

func TestClassifyProjectNotFoundPermanent(t *testing.T) {
	got := Classify(ErrProjectNotFound)
	if got.Code != CodeProjectNotFound || got.Retryable {
		t.Fatalf("Classify() = %#v", got)
	}
}

func TestClassifyUnknownPermanent(t *testing.T) {
	got := Classify(errors.New("unknown provider response"))
	if got.Code != CodeInternalAnalysisError || got.Retryable {
		t.Fatalf("Classify() = %#v", got)
	}
}
