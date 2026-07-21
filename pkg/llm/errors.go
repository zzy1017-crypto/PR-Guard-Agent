package llm

import "errors"

// 定义了与LLM交互时可能出现的错误类型，包括请求超时、无效JSON、无效风险报告和提供者错误等。
var (
	ErrLLMTimeout       = errors.New("llm request timeout")
	ErrLLMInvalidJSON   = errors.New("llm returned invalid json")
	ErrLLMInvalidReport = errors.New("llm returned invalid risk report")
	ErrLLMProvider      = errors.New("llm provider error")

	ErrInvalidReport = ErrLLMInvalidReport
)
