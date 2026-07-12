package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"pr-guard-agent/internal/config"
)

// New builds the single application logger. Application code should receive the
// returned logger through dependency injection rather than creating new loggers.
func New(cfg config.LoggerConfig) (*zap.Logger, error) {
	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(strings.TrimSpace(cfg.Level))); err != nil {
		return nil, fmt.Errorf("invalid logger level %q: %w", cfg.Level, err)
	}

	encoding := strings.ToLower(strings.TrimSpace(cfg.Encoding))
	if encoding == "" {
		encoding = "console"
	}
	if encoding != "console" && encoding != "json" {
		return nil, fmt.Errorf("invalid logger encoding %q", cfg.Encoding)
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder

	zapCfg := zap.Config{
		Level:            level,
		Development:      encoding == "console",
		Encoding:         encoding,
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stdout"},
	}
	return zapCfg.Build()
}
