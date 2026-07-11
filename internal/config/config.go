package config

import "github.com/spf13/viper"

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	MySQL       MySQLConfig       `mapstructure:"mysql"`
	Redis       RedisConfig       `mapstructure:"redis"`
	ReportCache ReportCacheConfig `mapstructure:"report_cache"`
	Qdrant      QdrantConfig      `mapstructure:"qdrant"`
	Embedding   EmbeddingConfig   `mapstructure:"embedding"`
	LLM         LLMConfig         `mapstructure:"llm"`
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
	v.SetDefault("mysql.host", "localhost")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.database", "pr_guard")
	v.SetDefault("mysql.username", "root")
	v.SetDefault("mysql.password", "123456")
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
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

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
