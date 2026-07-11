package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type AnalyzeResult struct {
	ReportID        uint     `json:"report_id"`
	ProjectID       uint     `json:"project_id"`
	DiffID          uint     `json:"diff_id"`
	RiskLevel       string   `json:"risk_level"`
	Summary         string   `json:"summary"`
	AffectedModules []string `json:"affected_modules"`
	PossibleRisks   []string `json:"possible_risks"`
	SuggestedTests  []string `json:"suggested_tests"`
	RelatedFiles    []string `json:"related_files"`
	RelatedSymbols  []string `json:"related_symbols"`
	Confidence      float64  `json:"confidence"`
	Cached          bool     `json:"cached"`
	Degraded        bool     `json:"degraded"`
	DegradedReason  string   `json:"degraded_reason,omitempty"`
}

type ReportCache struct {
	redisClient *redis.Client
	ttl         time.Duration
	enabled     bool
}

func NewReportCache(redisClient *redis.Client, ttl time.Duration, enabled bool) *ReportCache {
	return &ReportCache{
		redisClient: redisClient,
		ttl:         ttl,
		enabled:     enabled,
	}
}

func BuildReportCacheKey(projectID uint, codeVersionHash string, diffHash string) string {
	return fmt.Sprintf("prguard:report:%d:%s:%s", projectID, codeVersionHash, diffHash)
}

func (c *ReportCache) Enabled() bool {
	return c != nil && c.enabled
}

func (c *ReportCache) Get(ctx context.Context, key string) (*AnalyzeResult, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if c.redisClient == nil {
		return nil, errors.New("report cache redis client is nil")
	}

	value, err := c.redisClient.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get report cache failed: %w", err)
	}

	var result AnalyzeResult
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, fmt.Errorf("unmarshal report cache failed: %w", err)
	}
	return &result, nil
}

func (c *ReportCache) Set(ctx context.Context, key string, result *AnalyzeResult) error {
	if !c.Enabled() {
		return nil
	}
	if c.redisClient == nil {
		return errors.New("report cache redis client is nil")
	}
	if result == nil {
		return errors.New("report cache result is nil")
	}
	if result.Degraded {
		return errors.New("degraded report must not be cached")
	}

	value, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal report cache failed: %w", err)
	}
	if err := c.redisClient.Set(ctx, key, value, c.ttl).Err(); err != nil {
		return fmt.Errorf("set report cache failed: %w", err)
	}
	return nil
}
