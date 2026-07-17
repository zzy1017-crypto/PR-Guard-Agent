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
	if cfg.LLM.MockMode != "normal" || cfg.LLM.MockDelayMS != 1000 || cfg.LLM.TimeoutSeconds != 5 {
		t.Fatalf("unexpected LLM mock config: %#v", cfg.LLM)
	}
	if cfg.Logger.Level != "debug" || cfg.Logger.Encoding != "console" {
		t.Fatalf("unexpected logger config: %#v", cfg.Logger)
	}
	if !cfg.RateLimit.Enabled || cfg.RateLimit.Limit != 100 || cfg.RateLimit.WindowSeconds != 60 || !cfg.RateLimit.FailOpen {
		t.Fatalf("unexpected rate limit config: %#v", cfg.RateLimit)
	}
	if !cfg.AnalysisWorker.Enabled || cfg.AnalysisWorker.WorkerCount != 2 || cfg.AnalysisWorker.PollIntervalMS != 500 ||
		cfg.AnalysisWorker.TaskTimeoutSeconds != 30 || cfg.AnalysisWorker.StaleAfterSeconds != 60 || cfg.AnalysisWorker.MaxAttempts != 3 ||
		cfg.AnalysisWorker.RetryBaseSeconds != 2 || cfg.AnalysisWorker.RetryMaxSeconds != 8 || cfg.AnalysisWorker.RetryJitterPercent != 0 {
		t.Fatalf("unexpected analysis worker config: %#v", cfg.AnalysisWorker)
	}
	if !cfg.Ops.Enabled || cfg.Ops.DefaultPageSize != 20 || cfg.Ops.MaxPageSize != 100 ||
		cfg.Ops.DefaultMetricsWindowHours != 24 || cfg.Ops.MaxMetricsWindowHours != 168 ||
		cfg.Ops.QueryTimeoutSeconds != 3 {
		t.Fatalf("unexpected ops config: %#v", cfg.Ops)
	}
}

func TestLoadEnvironmentOverride(t *testing.T) {
	t.Setenv("PRGUARD_MYSQL_PASSWORD", "test-only-password")
	t.Setenv("PRGUARD_LLM_TIMEOUT_SECONDS", "17")

	configPath := filepath.Join("..", "..", "configs", "config.yaml")
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MySQL.Password != "test-only-password" {
		t.Fatalf("MySQL.Password was not loaded from the environment")
	}
	if cfg.LLM.TimeoutSeconds != 17 {
		t.Fatalf("LLM.TimeoutSeconds = %d, want 17", cfg.LLM.TimeoutSeconds)
	}
}

func TestOpsConfigValidation(t *testing.T) {
	valid := OpsConfig{
		Enabled: true, DefaultPageSize: 20, MaxPageSize: 100,
		DefaultMetricsWindowHours: 24, MaxMetricsWindowHours: 168,
		QueryTimeoutSeconds: 3,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config error = %v", err)
	}
	tests := []struct {
		name string
		edit func(*OpsConfig)
	}{
		{"default_page_size", func(c *OpsConfig) { c.DefaultPageSize = 0 }},
		{"max_page_size", func(c *OpsConfig) { c.MaxPageSize = 19 }},
		{"default_metrics_window", func(c *OpsConfig) { c.DefaultMetricsWindowHours = 0 }},
		{"max_metrics_window", func(c *OpsConfig) { c.MaxMetricsWindowHours = 23 }},
		{"query_timeout", func(c *OpsConfig) { c.QueryTimeoutSeconds = 0 }},
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

func TestAnalysisWorkerConfigValidation(t *testing.T) {
	valid := AnalysisWorkerConfig{
		Enabled: true, WorkerCount: 2, PollIntervalMS: 500,
		TaskTimeoutSeconds: 90, StaleAfterSeconds: 180, MaxAttempts: 3,
		RetryBaseSeconds: 2, RetryMaxSeconds: 30, RetryJitterPercent: 20,
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
		{"retry_base", func(c *AnalysisWorkerConfig) { c.RetryBaseSeconds = 0 }},
		{"retry_max", func(c *AnalysisWorkerConfig) { c.RetryMaxSeconds = 1 }},
		{"retry_jitter_low", func(c *AnalysisWorkerConfig) { c.RetryJitterPercent = -1 }},
		{"retry_jitter_high", func(c *AnalysisWorkerConfig) { c.RetryJitterPercent = 51 }},
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
