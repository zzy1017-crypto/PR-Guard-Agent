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
	if !cfg.AnalysisWorker.Enabled || cfg.AnalysisWorker.WorkerCount != 2 || cfg.AnalysisWorker.PollIntervalMS != 500 ||
		cfg.AnalysisWorker.TaskTimeoutSeconds != 90 || cfg.AnalysisWorker.StaleAfterSeconds != 180 || cfg.AnalysisWorker.MaxAttempts != 3 {
		t.Fatalf("unexpected analysis worker config: %#v", cfg.AnalysisWorker)
	}
}

func TestAnalysisWorkerConfigValidation(t *testing.T) {
	valid := AnalysisWorkerConfig{
		Enabled: true, WorkerCount: 2, PollIntervalMS: 500,
		TaskTimeoutSeconds: 90, StaleAfterSeconds: 180, MaxAttempts: 3,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config error = %v", err)
	}
	tests := []struct {
		name string
		edit func(*AnalysisWorkerConfig)
	}{
		{"worker_count_low", func(c *AnalysisWorkerConfig) { c.WorkerCount = 0 }},
		{"worker_count_high", func(c *AnalysisWorkerConfig) { c.WorkerCount = 21 }},
		{"poll_interval", func(c *AnalysisWorkerConfig) { c.PollIntervalMS = 0 }},
		{"task_timeout", func(c *AnalysisWorkerConfig) { c.TaskTimeoutSeconds = 0 }},
		{"stale_after", func(c *AnalysisWorkerConfig) { c.StaleAfterSeconds = 0 }},
		{"max_attempts", func(c *AnalysisWorkerConfig) { c.MaxAttempts = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := valid
			test.edit(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}
