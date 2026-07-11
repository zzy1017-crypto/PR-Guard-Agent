package cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestBuildReportCacheKey(t *testing.T) {
	got := BuildReportCacheKey(42, "code-hash", "diff-hash")
	want := "prguard:report:42:code-hash:diff-hash"
	if got != want {
		t.Fatalf("BuildReportCacheKey() = %q, want %q", got, want)
	}
}

func TestReportCacheGetMiss(t *testing.T) {
	client := newHookedRedisClient()
	client.AddHook(redisCommandHook{process: func(redis.Cmder) error {
		return redis.Nil
	}})

	result, err := NewReportCache(client, time.Hour, true).Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result != nil {
		t.Fatalf("Get() result = %#v, want nil", result)
	}
}

func TestReportCacheGetHit(t *testing.T) {
	want := AnalyzeResult{
		ReportID:   7,
		ProjectID:  2,
		DiffID:     3,
		RiskLevel:  "high",
		Summary:    "cache hit",
		Confidence: 0.9,
		Cached:     false,
	}
	value, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	client := newHookedRedisClient()
	client.AddHook(redisCommandHook{process: func(cmd redis.Cmder) error {
		stringCmd, ok := cmd.(*redis.StringCmd)
		if !ok {
			t.Fatalf("unexpected redis command type %T", cmd)
		}
		stringCmd.SetVal(string(value))
		return nil
	}})

	got, err := NewReportCache(client, time.Hour, true).Get(context.Background(), "hit")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.ReportID != want.ReportID || got.Summary != want.Summary || got.Cached {
		t.Fatalf("Get() result = %#v, want %#v", got, want)
	}
}

func TestReportCacheSetSerializesResult(t *testing.T) {
	var stored AnalyzeResult
	client := newHookedRedisClient()
	client.AddHook(redisCommandHook{process: func(cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) < 3 {
			t.Fatalf("unexpected SET args: %#v", args)
		}
		value, ok := args[2].([]byte)
		if !ok {
			t.Fatalf("SET value type = %T, want []byte", args[2])
		}
		if err := json.Unmarshal(value, &stored); err != nil {
			t.Fatalf("cached value is not valid JSON: %v", err)
		}
		statusCmd, ok := cmd.(*redis.StatusCmd)
		if !ok {
			t.Fatalf("unexpected redis command type %T", cmd)
		}
		statusCmd.SetVal("OK")
		return nil
	}})

	want := &AnalyzeResult{ReportID: 9, ProjectID: 1, DiffID: 4, Cached: false}
	if err := NewReportCache(client, time.Minute, true).Set(context.Background(), "key", want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if stored.ReportID != want.ReportID || stored.ProjectID != want.ProjectID || stored.Cached {
		t.Fatalf("stored result = %#v, want %#v", stored, want)
	}
}

func TestReportCacheReturnsRedisError(t *testing.T) {
	wantErr := errors.New("redis unavailable")
	client := newHookedRedisClient()
	client.AddHook(redisCommandHook{process: func(redis.Cmder) error {
		return wantErr
	}})

	_, err := NewReportCache(client, time.Hour, true).Get(context.Background(), "key")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Get() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestDisabledReportCacheDoesNotRequireRedis(t *testing.T) {
	cache := NewReportCache(nil, time.Hour, false)
	result, err := cache.Get(context.Background(), "key")
	if err != nil || result != nil {
		t.Fatalf("disabled Get() = (%#v, %v), want (nil, nil)", result, err)
	}
	if err := cache.Set(context.Background(), "key", &AnalyzeResult{}); err != nil {
		t.Fatalf("disabled Set() error = %v", err)
	}
}

func TestReportCacheRejectsDegradedResult(t *testing.T) {
	cache := NewReportCache(newHookedRedisClient(), time.Hour, true)
	err := cache.Set(context.Background(), "key", &AnalyzeResult{Degraded: true})
	if err == nil {
		t.Fatal("Set() error = nil, want degraded report rejection")
	}
}

type redisCommandHook struct {
	process func(redis.Cmder) error
}

func newHookedRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:            "unused",
		Protocol:        2,
		DisableIdentity: true,
		DialTimeout:     time.Millisecond,
		MaxRetries:      -1,
	})
}

func (h redisCommandHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h redisCommandHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		return h.process(cmd)
	}
}

func (h redisCommandHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
