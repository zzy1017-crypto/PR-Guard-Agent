package embedding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/taskerror"
)

const (
	ProviderMock     = "mock"
	defaultDimension = 1536
	defaultTimeout   = 10 * time.Second
	defaultBatchSize = 16
)

var (
	ErrEmbeddingTimeout       = taskerror.ErrEmbeddingTimeout
	ErrEmbeddingProviderError = taskerror.ErrEmbeddingProviderError
)

type Client struct {
	provider string
	baseURL  string
	apiKey   string
	model    string

	dimension int
	timeout   time.Duration
	batchSize int

	httpClient *http.Client
}

func NewClient(cfg config.EmbeddingConfig) *Client {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = ProviderMock
	}

	dimension := cfg.Dimension
	if dimension <= 0 {
		dimension = defaultDimension
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	return &Client{
		provider:   provider,
		baseURL:    strings.TrimSpace(cfg.BaseURL),
		apiKey:     strings.TrimSpace(cfg.APIKey),
		model:      strings.TrimSpace(cfg.Model),
		dimension:  dimension,
		timeout:    timeout,
		batchSize:  batchSize,
		httpClient: &http.Client{},
	}
}

func (c *Client) EmbedText(ctx context.Context, text string) ([]float32, error) {
	vectors, err := c.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("embedding response is empty")
	}
	return vectors[0], nil
}

func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errors.New("texts is empty")
	}
	for i, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("text at index %d is empty", i)
		}
	}

	results := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += c.batchSize {
		end := start + c.batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batchCtx, cancel := context.WithTimeout(ctx, c.timeout)
		batchVectors, err := c.embedBatch(batchCtx, texts[start:end])
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: %w", ErrEmbeddingTimeout, err)
			}
			return nil, err
		}
		results = append(results, batchVectors...)
	}

	return results, nil
}

func (c *Client) Provider() string {
	return c.provider
}

func (c *Client) IsMock() bool {
	return c.provider == ProviderMock
}

func (c *Client) Dimension() int {
	return c.dimension
}

func (c *Client) BatchSize() int {
	return c.batchSize
}

func (c *Client) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if c.provider == ProviderMock {
		return c.embedMockBatch(ctx, texts)
	}
	return c.embedHTTPBatch(ctx, texts)
}

func (c *Client) embedMockBatch(ctx context.Context, texts []string) ([][]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vectors = append(vectors, mockVector(text, c.dimension))
	}
	return vectors, nil
}

func (c *Client) embedHTTPBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if c.baseURL == "" {
		return nil, errors.New("embedding base_url is required")
	}

	body, err := json.Marshal(Request{
		Model: c.model,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %w", ErrEmbeddingTimeout, err)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
			return nil, fmt.Errorf("%w: %w", ErrEmbeddingProviderError, err)
		}
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusRequestTimeout {
			return nil, fmt.Errorf("%w: embedding api returned status %d", ErrEmbeddingTimeout, resp.StatusCode)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("%w: embedding api returned status %d", ErrEmbeddingProviderError, resp.StatusCode)
		}
		return nil, fmt.Errorf("embedding api returned status %d", resp.StatusCode)
	}

	var embeddingResp Response
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("decode embedding response failed: %w", err)
	}
	if len(embeddingResp.Data) != len(texts) {
		return nil, fmt.Errorf("embedding response count mismatch: got %d, want %d", len(embeddingResp.Data), len(texts))
	}

	vectors := make([][]float32, len(texts))
	for i, item := range embeddingResp.Data {
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("embedding response at index %d is empty", i)
		}

		resultIndex := i
		if item.Index != nil {
			if *item.Index < 0 || *item.Index >= len(texts) {
				return nil, fmt.Errorf("embedding response index out of range: %d", *item.Index)
			}
			resultIndex = *item.Index
		}
		vectors[resultIndex] = item.Embedding
	}

	for i, vector := range vectors {
		if len(vector) == 0 {
			return nil, fmt.Errorf("embedding response missing vector at index %d", i)
		}
	}

	return vectors, nil
}

func mockVector(text string, dimension int) []float32 {
	vector := make([]float32, dimension)
	blockNo := uint64(0)
	pos := 0

	for pos < dimension {
		hash := sha256.New()
		_, _ = hash.Write([]byte(text))

		var counter [8]byte
		binary.BigEndian.PutUint64(counter[:], blockNo)
		_, _ = hash.Write(counter[:])

		block := hash.Sum(nil)
		for offset := 0; offset+4 <= len(block) && pos < dimension; offset += 4 {
			raw := binary.BigEndian.Uint32(block[offset : offset+4])
			vector[pos] = float32((float64(raw)/float64(math.MaxUint32))*2 - 1)
			pos++
		}
		blockNo++
	}

	return vector
}
