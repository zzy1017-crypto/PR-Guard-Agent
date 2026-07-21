package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"pr-guard-agent/internal/config"
)

const (
	ProviderMock       = "mock"
	defaultModel       = "mock-llm"
	defaultTimeout     = 20 * time.Second
	defaultMaxTokens   = 1200
	defaultTemperature = 0.2
)

type Client struct {
	provider    string
	mockMode    string
	mockDelay   time.Duration
	baseURL     string
	apiKey      string
	model       string
	timeout     time.Duration
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

// NewClient 创建一个新的 LLM 客户端实例，使用提供的配置进行初始化。
func NewClient(cfg config.LLMConfig) *Client {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = ProviderMock
	}

	// 1. 设置默认模型，如果配置中未指定模型，则使用默认模型。
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	// 2. 设置请求超时时间，如果配置中未指定超时时间，则使用默认超时时间。
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	// 3. 设置最大令牌数，如果配置中未指定最大令牌数，则使用默认最大令牌数。
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	// 4. 设置温度参数，如果配置中未指定温度，则使用默认温度。
	temperature := cfg.Temperature
	if temperature < 0 {
		temperature = defaultTemperature
	}

	// 5. 返回一个新的 Client 实例，包含所有配置参数。
	return &Client{
		provider:    provider,
		mockMode:    strings.ToLower(strings.TrimSpace(cfg.MockMode)),
		mockDelay:   time.Duration(cfg.MockDelayMS) * time.Millisecond,
		baseURL:     strings.TrimSpace(cfg.BaseURL),
		apiKey:      strings.TrimSpace(cfg.APIKey),
		model:       model,
		timeout:     timeout,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{},
	}
}

// Generate 使用 LLM 客户端生成响应，基于提供的上下文和提示字符串。
// 它会根据客户端的配置选择使用 Mock 模式或 HTTP 请求模式。
// 如果客户端或上下文为 nil，或者提示字符串为空，将返回错误。
// 如果请求超时或发生其他错误，将返回相应的错误信息。
func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {

	// 0. 检查客户端、上下文和提示字符串是否有效，如果无效则返回错误。
	if c == nil {
		return "", errors.New("llm client is nil")
	}
	if ctx == nil {
		return "", errors.New("llm context is nil")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("llm prompt is empty")
	}

	// 1. 设置请求超时时间，如果客户端未指定超时时间，则使用默认超时时间。
	timeout := c.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	// 2. 创建一个带有超时的上下文，用于控制请求的生命周期。
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 3. 根据客户端的提供者类型选择使用 Mock 模式或 HTTP 请求模式。
	if c.provider == ProviderMock {
		return c.generateMock(reqCtx)
	}
	// 4. 使用 HTTP 请求模式生成响应，调用 generateHTTP 方法。
	output, err := c.generateHTTP(reqCtx, prompt)
	if err != nil {
		return "", classifyClientError(reqCtx, err)
	}
	return output, nil
}

// Provider 返回客户端的提供者类型，如果客户端为 nil，则返回空字符串。
func (c *Client) Provider() string {
	if c == nil {
		return ""
	}
	return c.provider
}

// IsMock 返回客户端是否处于 Mock 模式，如果客户端为 nil，则返回 false。
func (c *Client) IsMock() bool {
	return c != nil && c.provider == ProviderMock
}

// generateMock 使用 Mock 模式生成响应，模拟 LLM 的行为。
// 它会根据客户端的 mockMode 配置返回不同的模拟响应，包括有效 JSON、无效 JSON 或带有虚构源的报告。
// 如果请求超时或上下文被取消，将返回相应的错误信息。
func (c *Client) generateMock(ctx context.Context) (string, error) {

	// 1. 设置模拟延迟时间，如果客户端未指定模拟延迟，则使用默认延迟时间。
	delay := c.mockDelay
	if delay < 0 {
		delay = 0
	}
	// 2. 创建一个定时器，用于模拟请求的延迟。
	timer := time.NewTimer(delay)
	defer timer.Stop()

	// 3. 使用 select 语句等待定时器触发或上下文取消。
	select {
	case <-ctx.Done():
		return "", classifyClientError(ctx, ctx.Err())
	case <-timer.C:
	}

	// 4. 根据客户端的 mockMode 配置返回不同的模拟响应。
	valid := `{"risk_level":"medium","summary":"Mock risk analysis completed.","affected_modules":[],"possible_risks":["Review the changed behavior and its failure paths."],"suggested_tests":["Run focused regression tests for the changed code."],"related_files":[],"related_symbols":[],"confidence":0.78}`
	switch c.mockMode {
	case "", "normal":
		return valid, nil
	case "marksown_json", "markdown_json":
		return "```json\n" + valid + "\n```", nil
	case "invalid_json":
		return "this is not valid json", nil
	case "invented_source":
		return `{"risk_level":"medium","summary":"Mock report with invented sources.","affected_modules":[],"possible_risks":[],"suggested_tests":[],"related_files":["invented/file.go"],"related_symbols":["Invented.Symbol"],"confidence":0.5}`, nil
	default:
		return "", fmt.Errorf("%w: unsupported mock_mode %q", ErrLLMProvider, c.mockMode)
	}
}

// generateHTTP 使用 HTTP 请求模式生成响应，向 LLM 提供者发送请求并解析响应。
// 它会根据客户端的配置构建请求体，包括模型、提示、消息、最大令牌数和温度等字段。
// 如果请求失败或响应无效，将返回相应的错误信息。
func (c *Client) generateHTTP(ctx context.Context, prompt string) (string, error) {

	// 1. 检查客户端的 baseURL 是否为空，如果为空则返回错误。
	if c.baseURL == "" {
		return "", fmt.Errorf("%w: base_url is required", ErrLLMProvider)
	}

	// 2. 构建请求体，将模型、提示、消息、最大令牌数和温度等字段序列化为 JSON。
	body, err := json.Marshal(Request{
		Model:  c.model,
		Prompt: prompt,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshal llm request failed: %w", err)
	}

	// 3. 创建一个 HTTP POST 请求，设置请求头和请求体，如果创建请求失败则返回错误。
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create llm request failed: %w", err)
	}
	// 4. 设置请求头，包括 Content-Type 和 Authorization，如果客户端的 apiKey 不为空，则设置 Bearer Token。
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// 5. 使用客户端的 httpClient 发送请求，如果未指定 httpClient，则使用默认的 http.Client。
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	// 6. 发送请求并获取响应，如果请求失败则返回错误。
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	// 7. 检查响应的状态码，如果状态码不在 200-299 范围内，则返回错误。
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("%w: api returned status %d", ErrLLMProvider, resp.StatusCode)
	}

	// 8. 解析响应体为 Response 结构体，如果解析失败则返回错误。
	var llmResp Response
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", fmt.Errorf("%w: decode response failed: %w", ErrLLMProvider, err)
	}

	// 9. 提取响应文本，如果响应文本为空，则返回错误。
	output := extractResponseText(llmResp)
	if strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("%w: response text is empty", ErrLLMProvider)
	}
	return output, nil
}

// classifyClientError 根据上下文和错误类型对客户端错误进行分类，返回更具体的错误信息。
// 它会检查错误是否为超时、取消或提供者错误，并返回相应的错误类型。
// 如果错误为 nil，则返回 nil。
func classifyClientError(ctx context.Context, err error) error {

	// 0. 如果错误为 nil，则直接返回 nil。
	if err == nil {
		return nil
	}
	// 1. 检查错误是否为 ErrLLMTimeout 或 ErrLLMProvider，如果是则直接返回该错误。
	if errors.Is(err, ErrLLMTimeout) || errors.Is(err, ErrLLMProvider) {
		return err
	}
	// 2. 检查上下文是否超时，如果是则返回 ErrLLMTimeout 错误。
	if errors.Is(err, context.DeadlineExceeded) || (ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return fmt.Errorf("%w: %w", ErrLLMTimeout, context.DeadlineExceeded)
	}
	// 3. 检查上下文是否被取消，如果是则返回提供者错误。
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("llm request canceled: %w", err)
	}
	// 4. 对于其他错误，返回提供者错误，并附加原始错误信息。
	return fmt.Errorf("%w: %w", ErrLLMProvider, err)
}

// extractResponseText 从 LLM 响应中提取文本内容，优先返回 output、content、text 或 choices 中的消息内容。
// 如果所有字段都为空，则返回空字符串。
func extractResponseText(resp Response) string {

	// 0. 检查响应的 output、content 和 text 字段，如果不为空则返回相应的内容。
	if strings.TrimSpace(resp.Output) != "" {
		return resp.Output
	}
	if strings.TrimSpace(resp.Content) != "" {
		return resp.Content
	}
	if strings.TrimSpace(resp.Text) != "" {
		return resp.Text
	}

	// 1. 遍历响应的 choices 切片，检查每个 choice 的 message 和 text 字段，如果不为空则返回相应的内容。
	for _, choice := range resp.Choices {
		if choice.Message != nil && strings.TrimSpace(choice.Message.Content) != "" {
			return choice.Message.Content
		}
		if strings.TrimSpace(choice.Text) != "" {
			return choice.Text
		}
	}

	// 2. 如果所有字段都为空，则返回空字符串。
	return ""
}
