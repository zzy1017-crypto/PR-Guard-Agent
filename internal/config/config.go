package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Logger         LoggerConfig         `mapstructure:"logger"`
	MySQL          MySQLConfig          `mapstructure:"mysql"`
	Redis          RedisConfig          `mapstructure:"redis"`
	RateLimit      RateLimitConfig      `mapstructure:"rate_limit"`
	ReportCache    ReportCacheConfig    `mapstructure:"report_cache"`
	Qdrant         QdrantConfig         `mapstructure:"qdrant"`
	Embedding      EmbeddingConfig      `mapstructure:"embedding"`
	LLM            LLMConfig            `mapstructure:"llm"`
	AnalysisWorker AnalysisWorkerConfig `mapstructure:"analysis_worker"`
}

type AnalysisWorkerConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	WorkerCount        int  `mapstructure:"worker_count"`
	PollIntervalMS     int  `mapstructure:"poll_interval_ms"`
	TaskTimeoutSeconds int  `mapstructure:"task_timeout_seconds"`
	StaleAfterSeconds  int  `mapstructure:"stale_after_seconds"`
	MaxAttempts        int  `mapstructure:"max_attempts"`
}

func (c AnalysisWorkerConfig) Validate() error {
	if c.WorkerCount < 1 || c.WorkerCount > 20 {
		return fmt.Errorf("analysis_worker.worker_count must be between 1 and 20")
	}
	if c.PollIntervalMS <= 0 {
		return fmt.Errorf("analysis_worker.poll_interval_ms must be greater than 0")
	}
	if c.TaskTimeoutSeconds <= 0 {
		return fmt.Errorf("analysis_worker.task_timeout_seconds must be greater than 0")
	}
	if c.StaleAfterSeconds <= 0 {
		return fmt.Errorf("analysis_worker.stale_after_seconds must be greater than 0")
	}
	if c.MaxAttempts <= 0 {
		return fmt.Errorf("analysis_worker.max_attempts must be greater than 0")
	}
	return nil
}

type LoggerConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RateLimitConfig struct {
	Enabled       bool  `mapstructure:"enabled"`
	Limit         int64 `mapstructure:"limit"`
	WindowSeconds int64 `mapstructure:"window_seconds"`
	FailOpen      bool  `mapstructure:"fail_open"`
}

type ReportCacheConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	TTLSeconds int  `mapstructure:"ttl_seconds"`
}

type QdrantConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	CollectionName string `mapstructure:"collection_name"`
	VectorSize     int    `mapstructure:"vector_size"`
	Distance       string `mapstructure:"distance"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type EmbeddingConfig struct {
	Provider       string `mapstructure:"provider"`
	BaseURL        string `mapstructure:"base_url"`
	APIKey         string `mapstructure:"api_key"`
	Model          string `mapstructure:"model"`
	Dimension      int    `mapstructure:"dimension"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	BatchSize      int    `mapstructure:"batch_size"`
}

type LLMConfig struct {
	Provider       string  `mapstructure:"provider"`
	MockMode       string  `mapstructure:"mock_mode"`
	MockDelayMS    int     `mapstructure:"mock_delay_ms"`
	BaseURL        string  `mapstructure:"base_url"`
	APIKey         string  `mapstructure:"api_key"`
	Model          string  `mapstructure:"model"`
	TimeoutSeconds int     `mapstructure:"timeout_seconds"`
	MaxTokens      int     `mapstructure:"max_tokens"`
	Temperature    float64 `mapstructure:"temperature"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("logger.level", "debug")
	v.SetDefault("logger.encoding", "console")
	v.SetDefault("mysql.host", "localhost")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.database", "pr_guard")
	v.SetDefault("mysql.username", "root")
	v.SetDefault("mysql.password", "123456")
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.limit", 10)
	v.SetDefault("rate_limit.window_seconds", 60)
	v.SetDefault("rate_limit.fail_open", true)
	v.SetDefault("report_cache.enabled", true)
	v.SetDefault("report_cache.ttl_seconds", 3600)
	v.SetDefault("qdrant.host", "localhost")
	v.SetDefault("qdrant.port", 6334)
	v.SetDefault("qdrant.collection_name", "pr_guard_code_chunks")
	v.SetDefault("qdrant.vector_size", 1536)
	v.SetDefault("qdrant.distance", "Cosine")
	v.SetDefault("qdrant.timeout_seconds", 10)
	v.SetDefault("embedding.provider", "mock")
	v.SetDefault("embedding.base_url", "")
	v.SetDefault("embedding.api_key", "")
	v.SetDefault("embedding.model", "mock_embedding")
	v.SetDefault("embedding.dimension", 1536)
	v.SetDefault("embedding.timeout_seconds", 10)
	v.SetDefault("embedding.batch_size", 16)
	v.SetDefault("llm.provider", "mock")
	v.SetDefault("llm.mock_mode", "normal")
	v.SetDefault("llm.mock_delay_ms", 0)
	v.SetDefault("llm.base_url", "")
	v.SetDefault("llm.api_key", "")
	v.SetDefault("llm.model", "mock-llm")
	v.SetDefault("llm.timeout_seconds", 20)
	v.SetDefault("llm.max_tokens", 1200)
	v.SetDefault("llm.temperature", 0.2)
	v.SetDefault("analysis_worker.enabled", true)
	v.SetDefault("analysis_worker.worker_count", 2)
	v.SetDefault("analysis_worker.poll_interval_ms", 500)
	v.SetDefault("analysis_worker.task_timeout_seconds", 90)
	v.SetDefault("analysis_worker.stale_after_seconds", 180)
	v.SetDefault("analysis_worker.max_attempts", 3)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.AnalysisWorker.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}
