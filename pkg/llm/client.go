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

func NewClient(cfg config.LLMConfig) *Client {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = ProviderMock
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	temperature := cfg.Temperature
	if temperature < 0 {
		temperature = defaultTemperature
	}

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

func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	if c == nil {
		return "", errors.New("llm client is nil")
	}
	if ctx == nil {
		return "", errors.New("llm context is nil")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("llm prompt is empty")
	}

	timeout := c.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if c.provider == ProviderMock {
		return c.generateMock(reqCtx)
	}
	output, err := c.generateHTTP(reqCtx, prompt)
	if err != nil {
		return "", classifyClientError(reqCtx, err)
	}
	return output, nil
}

func (c *Client) Provider() string {
	if c == nil {
		return ""
	}
	return c.provider
}

func (c *Client) IsMock() bool {
	return c != nil && c.provider == ProviderMock
}

func (c *Client) generateMock(ctx context.Context) (string, error) {
	delay := c.mockDelay
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return "", classifyClientError(ctx, ctx.Err())
	case <-timer.C:
	}

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

func (c *Client) generateHTTP(ctx context.Context, prompt string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("%w: base_url is required", ErrLLMProvider)
	}

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create llm request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("%w: api returned status %d: %s", ErrLLMProvider, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var llmResp Response
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", fmt.Errorf("%w: decode response failed: %w", ErrLLMProvider, err)
	}

	output := extractResponseText(llmResp)
	if strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("%w: response text is empty", ErrLLMProvider)
	}
	return output, nil
}

func classifyClientError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrLLMTimeout) || errors.Is(err, ErrLLMProvider) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || (ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return fmt.Errorf("%w: %w", ErrLLMTimeout, context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("llm request canceled: %w", err)
	}
	return fmt.Errorf("%w: %w", ErrLLMProvider, err)
}

func extractResponseText(resp Response) string {
	if strings.TrimSpace(resp.Output) != "" {
		return resp.Output
	}
	if strings.TrimSpace(resp.Content) != "" {
		return resp.Content
	}
	if strings.TrimSpace(resp.Text) != "" {
		return resp.Text
	}

	for _, choice := range resp.Choices {
		if choice.Message != nil && strings.TrimSpace(choice.Message.Content) != "" {
			return choice.Message.Content
		}
		if strings.TrimSpace(choice.Text) != "" {
			return choice.Text
		}
	}

	return ""
}
