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
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("llm prompt is empty")
	}
	if c == nil {
		return "", errors.New("llm client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
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
	return c.generateHTTP(reqCtx, prompt)
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
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	return `{"risk_level":"medium","summary":"CreateOrder now depends on stock deduction before order persistence; verify transaction boundaries and failure handling.","affected_modules":["order","stock"],"possible_risks":["Stock may be deducted while order creation later fails, causing inconsistent inventory.","Context is limited, so rollback or transaction handling could not be fully verified."],"suggested_tests":["Test CreateOrder returns an error and does not create an order when DeductStock fails.","Test inventory remains consistent when stock deduction succeeds but order persistence fails."],"related_files":["internal/service/order_service.go","internal/service/stock_service.go"],"related_symbols":["OrderService.CreateOrder","StockService.DeductStock"],"confidence":0.78}`, nil
}

func (c *Client) generateHTTP(ctx context.Context, prompt string) (string, error) {
	if c.baseURL == "" {
		return "", errors.New("llm base_url is required")
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
		return "", fmt.Errorf("llm api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var llmResp Response
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", fmt.Errorf("decode llm response failed: %w", err)
	}

	output := extractResponseText(llmResp)
	if strings.TrimSpace(output) == "" {
		return "", errors.New("llm response text is empty")
	}
	return output, nil
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
