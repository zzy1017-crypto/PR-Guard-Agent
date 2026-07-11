package llm

import "errors"

var (
	ErrLLMTimeout       = errors.New("llm request timeout")
	ErrLLMInvalidJSON   = errors.New("llm returned invalid json")
	ErrLLMInvalidReport = errors.New("llm returned invalid risk report")
	ErrLLMProvider      = errors.New("llm provider error")

	// ErrInvalidReport is kept as an alias for callers using the shorter name.
	ErrInvalidReport = ErrLLMInvalidReport
)
