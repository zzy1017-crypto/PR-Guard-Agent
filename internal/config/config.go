package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// 定义了一个Config结构体，用于存储应用程序的配置参数。该结构体包含了多个嵌套的结构体，每个嵌套结构体对应一个配置模块，如ServerConfig、LoggerConfig、MySQLConfig等。
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

// 定义了一个AnalysisWorkerConfig结构体，用于存储分析任务工作者的配置参数。该结构体包含了多个字段，如Enabled、WorkerCount、PollIntervalMS等，用于控制分析任务工作者的行为。
type AnalysisWorkerConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	WorkerCount        int  `mapstructure:"worker_count"`
	PollIntervalMS     int  `mapstructure:"poll_interval_ms"`
	TaskTimeoutSeconds int  `mapstructure:"task_timeout_seconds"`
	StaleAfterSeconds  int  `mapstructure:"stale_after_seconds"`
	MaxAttempts        int  `mapstructure:"max_attempts"`
	RetryBaseSeconds   int  `mapstructure:"retry_base_seconds"`
	RetryMaxSeconds    int  `mapstructure:"retry_max_seconds"`
	RetryJitterPercent int  `mapstructure:"retry_jitter_percent"`
}

// Validate方法用于验证AnalysisWorkerConfig结构体的字段值是否合法。如果字段值不合法，则返回一个错误。
func (c AnalysisWorkerConfig) Validate() error {

	// 验证分析任务工作者的配置参数是否合法
	if c.WorkerCount < 1 || c.WorkerCount > 20 {
		return fmt.Errorf("analysis_worker.worker_count must be between 1 and 20")
	}

	// 验证分析任务工作者的轮询间隔是否大于0
	if c.PollIntervalMS <= 0 {
		return fmt.Errorf("analysis_worker.poll_interval_ms must be greater than 0")
	}

	// 验证分析任务工作者的任务超时时间是否大于0
	if c.TaskTimeoutSeconds <= 0 {
		return fmt.Errorf("analysis_worker.task_timeout_seconds must be greater than 0")
	}

	// 验证分析任务工作者的过期时间是否大于0
	if c.StaleAfterSeconds <= 0 {
		return fmt.Errorf("analysis_worker.stale_after_seconds must be greater than 0")
	}

	// 验证分析任务工作者的最大尝试次数是否大于0
	if c.MaxAttempts <= 0 {
		return fmt.Errorf("analysis_worker.max_attempts must be greater than 0")
	}
	if c.RetryBaseSeconds <= 0 {
		return fmt.Errorf("analysis_worker.retry_base_seconds must be greater than 0")
	}
	if c.RetryMaxSeconds < c.RetryBaseSeconds {
		return fmt.Errorf("analysis_worker.retry_max_seconds must be greater than or equal to retry_base_seconds")
	}
	if c.RetryJitterPercent < 0 || c.RetryJitterPercent > 50 {
		return fmt.Errorf("analysis_worker.retry_jitter_percent must be between 0 and 50")
	}
	return nil
}

// LoggerConfig结构体用于存储日志记录器的配置参数。该结构体包含了两个字段：Level和Encoding，分别用于指定日志级别和日志编码格式。
type LoggerConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

// ServerConfig结构体用于存储服务器的配置参数。该结构体包含了两个字段：Port和Mode，分别用于指定服务器的端口号和运行模式。
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// MySQLConfig结构体用于存储MySQL数据库的配置参数。该结构体包含了五个字段：Host、Port、Database、Username和Password，分别用于指定MySQL数据库的主机地址、端口号、数据库名称、用户名和密码。
type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// RedisConfig结构体用于存储Redis数据库的配置参数。该结构体包含了三个字段：Addr、Password和DB，分别用于指定Redis数据库的地址、密码和数据库编号。
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RateLimitConfig结构体用于存储速率限制器的配置参数。该结构体包含了四个字段：Enabled、Limit、WindowSeconds和FailOpen，分别用于指定速率限制器是否启用、请求限制次数、时间窗口长度和是否在Redis不可用时允许请求通过。
type RateLimitConfig struct {
	Enabled       bool  `mapstructure:"enabled"`
	Limit         int64 `mapstructure:"limit"`
	WindowSeconds int64 `mapstructure:"window_seconds"`
	FailOpen      bool  `mapstructure:"fail_open"`
}

// ReportCacheConfig结构体用于存储报告缓存的配置参数。该结构体包含了两个字段：Enabled和TTLSeconds，分别用于指定报告缓存是否启用和缓存的过期时间（以秒为单位）。
type ReportCacheConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	TTLSeconds int  `mapstructure:"ttl_seconds"`
}

// QdrantConfig结构体用于存储Qdrant向量数据库的配置参数。该结构体包含了五个字段：Host、Port、CollectionName、VectorSize和Distance，分别用于指定Qdrant数据库的主机地址、端口号、集合名称、向量维度和距离度量方式。
type QdrantConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	CollectionName string `mapstructure:"collection_name"`
	VectorSize     int    `mapstructure:"vector_size"`
	Distance       string `mapstructure:"distance"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// EmbeddingConfig结构体用于存储嵌入模型的配置参数。该结构体包含了六个字段：Provider、BaseURL、APIKey、Model、Dimension和TimeoutSeconds，分别用于指定嵌入模型的提供商、基础URL、API密钥、模型名称、向量维度和请求超时时间（以秒为单位）。
type EmbeddingConfig struct {
	Provider       string `mapstructure:"provider"`
	BaseURL        string `mapstructure:"base_url"`
	APIKey         string `mapstructure:"api_key"`
	Model          string `mapstructure:"model"`
	Dimension      int    `mapstructure:"dimension"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	BatchSize      int    `mapstructure:"batch_size"`
}

// LLMConfig结构体用于存储大语言模型（LLM）的配置参数。该结构体包含了九个字段：Provider、MockMode、MockDelayMS、BaseURL、APIKey、Model、TimeoutSeconds、MaxTokens和Temperature，分别用于指定大语言模型的提供商、模拟模式、模拟延迟时间（以毫秒为单位）、基础URL、API密钥、模型名称、请求超时时间（以秒为单位）、最大令牌数和温度参数（输出随机性）。
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

// Load函数用于从指定路径加载配置文件，并将其解析为Config结构体实例。该函数使用Viper库来读取和解析配置文件，并提供了一些默认值，以确保在配置文件中缺少某些字段时，程序仍能正常运行。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)   // 设置配置文件路径
	v.SetConfigType("yaml") // 设置配置文件类型为YAML

	v.SetDefault("server.port", 8080)                              // 设置服务器端口的默认值为8080
	v.SetDefault("server.mode", "debug")                           // 设置服务器运行模式的默认值为debug
	v.SetDefault("logger.level", "debug")                          // 设置日志记录器的默认日志级别为debug
	v.SetDefault("logger.encoding", "console")                     // 设置日志记录器的默认编码格式为console
	v.SetDefault("mysql.host", "localhost")                        // 设置MySQL数据库主机的默认值为localhost
	v.SetDefault("mysql.port", 3306)                               // 设置MySQL数据库端口的默认值为3306
	v.SetDefault("mysql.database", "pr_guard")                     // 设置MySQL数据库名称的默认值为pr_guard
	v.SetDefault("mysql.username", "root")                         // 设置MySQL数据库用户名的默认值为root
	v.SetDefault("mysql.password", "123456")                       // 设置MySQL数据库密码的默认值为123456
	v.SetDefault("redis.addr", "localhost:6379")                   // 设置Redis数据库地址的默认值为localhost:6379
	v.SetDefault("redis.password", "")                             // 设置Redis数据库密码的默认值为空字符串
	v.SetDefault("redis.db", 0)                                    // 设置Redis数据库编号的默认值为0
	v.SetDefault("rate_limit.enabled", true)                       // 设置速率限制器启用的默认值为true
	v.SetDefault("rate_limit.limit", 10)                           // 设置速率限制器请求限制次数的默认值为10
	v.SetDefault("rate_limit.window_seconds", 60)                  // 设置速率限制器时间窗口长度的默认值为60秒
	v.SetDefault("rate_limit.fail_open", true)                     // 设置速率限制器在Redis不可用时允许请求通过的默认值为true
	v.SetDefault("report_cache.enabled", true)                     // 设置报告缓存启用的默认值为true
	v.SetDefault("report_cache.ttl_seconds", 3600)                 // 设置报告缓存过期时间的默认值为3600秒（1小时）
	v.SetDefault("qdrant.host", "localhost")                       // 设置Qdrant数据库主机的默认值为localhost
	v.SetDefault("qdrant.port", 6334)                              // 设置Qdrant数据库端口的默认值为6334
	v.SetDefault("qdrant.collection_name", "pr_guard_code_chunks") // 设置Qdrant数据库集合名称的默认值为pr_guard_code_chunks
	v.SetDefault("qdrant.vector_size", 1536)                       // 设置Qdrant数据库向量维度的默认值为1536
	v.SetDefault("qdrant.distance", "Cosine")                      // 设置Qdrant数据库距离度量方式的默认值为Cosine
	v.SetDefault("qdrant.timeout_seconds", 10)                     // 设置Qdrant数据库请求超时时间的默认值为10秒
	v.SetDefault("embedding.provider", "mock")                     // 设置嵌入模型提供商的默认值为mock
	v.SetDefault("embedding.base_url", "")                         // 设置嵌入模型基础URL的默认值为空字符串
	v.SetDefault("embedding.api_key", "")                          // 设置嵌入模型API密钥的默认值为空字符串
	v.SetDefault("embedding.model", "mock_embedding")              // 设置嵌入模型名称的默认值为mock_embedding
	v.SetDefault("embedding.dimension", 1536)                      // 设置嵌入模型向量维度的默认值为1536
	v.SetDefault("embedding.timeout_seconds", 10)                  // 设置嵌入模型请求超时时间的默认值为10秒
	v.SetDefault("embedding.batch_size", 16)                       // 设置嵌入模型批处理大小的默认值为16
	v.SetDefault("llm.provider", "mock")                           // 设置大语言模型提供商的默认值为mock
	v.SetDefault("llm.mock_mode", "normal")                        // 设置大语言模型模拟模式的默认值为normal
	v.SetDefault("llm.mock_delay_ms", 0)                           // 设置大语言模型模拟延迟时间的默认值为0毫秒
	v.SetDefault("llm.base_url", "")                               // 设置大语言模型基础URL的默认值为空字符串
	v.SetDefault("llm.api_key", "")                                // 设置大语言模型API密钥的默认值为空字符串
	v.SetDefault("llm.model", "mock-llm")                          // 设置大语言模型名称的默认值为mock-llm
	v.SetDefault("llm.timeout_seconds", 20)                        // 设置大语言模型请求超时时间的默认值为20秒
	v.SetDefault("llm.max_tokens", 1200)                           // 设置大语言模型最大令牌数的默认值为1200
	v.SetDefault("llm.temperature", 0.2)                           // 设置大语言模型温度参数的默认值为0.2
	v.SetDefault("analysis_worker.enabled", true)                  // 设置分析任务工作者启用的默认值为true
	v.SetDefault("analysis_worker.worker_count", 2)                // 设置分析任务工作者数量的默认值为2
	v.SetDefault("analysis_worker.poll_interval_ms", 500)          // 设置分析任务工作者轮询间隔的默认值为500毫秒
	v.SetDefault("analysis_worker.task_timeout_seconds", 90)       // 设置分析任务工作者任务超时时间的默认值为90秒
	v.SetDefault("analysis_worker.stale_after_seconds", 180)       // 设置分析任务工作者过期时间的默认值为180秒
	v.SetDefault("analysis_worker.max_attempts", 3)                // 设置分析任务工作者最大尝试次数的默认值为3
	v.SetDefault("analysis_worker.retry_base_seconds", 2)
	v.SetDefault("analysis_worker.retry_max_seconds", 30)
	v.SetDefault("analysis_worker.retry_jitter_percent", 20)

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config // 创建一个Config结构体实例，用于存储解析后的配置参数

	// 将读取到的配置参数映射到Config结构体实例中
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// 验证分析任务工作者的配置参数是否合法
	if err := cfg.AnalysisWorker.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}
