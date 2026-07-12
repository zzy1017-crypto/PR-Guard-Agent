package config

import (
	"path/filepath"
	"testing"
)

func TestLoadReportCacheConfig(t *testing.T) {
	configPath := filepath.Join("..", "..", "configs", "config.yaml")
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.ReportCache.Enabled {
		t.Fatal("ReportCache.Enabled = false, want true")
	}
	if cfg.ReportCache.TTLSeconds != 3600 {
		t.Fatalf("ReportCache.TTLSeconds = %d, want 3600", cfg.ReportCache.TTLSeconds)
	}
	if cfg.LLM.MockMode != "normal" || cfg.LLM.MockDelayMS != 0 || cfg.LLM.TimeoutSeconds != 2 {
		t.Fatalf("unexpected LLM mock config: %#v", cfg.LLM)
	}
	if cfg.Logger.Level != "debug" || cfg.Logger.Encoding != "console" {
		t.Fatalf("unexpected logger config: %#v", cfg.Logger)
	}
	if !cfg.RateLimit.Enabled || cfg.RateLimit.Limit != 10 || cfg.RateLimit.WindowSeconds != 60 || !cfg.RateLimit.FailOpen {
		t.Fatalf("unexpected rate limit config: %#v", cfg.RateLimit)
	}
}
