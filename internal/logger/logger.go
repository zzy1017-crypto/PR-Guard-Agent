package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"pr-guard-agent/internal/config"
)

// 创建一个新的zap.Logger实例，使用提供的配置进行初始化。
func New(cfg config.LoggerConfig) (*zap.Logger, error) {
	level := zap.NewAtomicLevel() // 创建一个新的zap.AtomicLevel实例，用于动态调整日志级别。

	// 将配置中的日志级别字符串解析为zapcore.Level类型，并设置到level中。
	if err := level.UnmarshalText([]byte(strings.TrimSpace(cfg.Level))); err != nil {
		return nil, fmt.Errorf("invalid logger level %q: %w", cfg.Level, err)
	}

	// 将配置中的日志编码格式进行处理，默认为"console"。如果配置中指定了其他编码格式，则进行验证。
	encoding := strings.ToLower(strings.TrimSpace(cfg.Encoding))
	if encoding == "" {
		encoding = "console"
	}
	if encoding != "console" && encoding != "json" {
		return nil, fmt.Errorf("invalid logger encoding %q", cfg.Encoding)
	}

	encoderCfg := zap.NewProductionEncoderConfig()         // 创建一个新的zap.EncoderConfig实例，用于配置日志编码器的行为。
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder     // 设置时间编码器为ISO8601格式。
	encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder // 设置日志级别编码器为小写字母格式。

	// 创建一个新的zap.Config实例，使用之前设置的日志级别、编码格式和编码器配置进行初始化。
	zapCfg := zap.Config{
		Level:            level,
		Development:      encoding == "console",
		Encoding:         encoding,
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stdout"},
	}
	return zapCfg.Build() // 构建并返回一个新的zap.Logger实例。
}
