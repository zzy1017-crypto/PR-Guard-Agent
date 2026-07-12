package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	appLogger "pr-guard-agent/internal/logger"
	"pr-guard-agent/internal/router"
	reportcache "pr-guard-agent/pkg/cache"
)

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Printf("load config failed: %v", err)
		return
	}
	logger, err := appLogger.New(cfg.Logger)
	if err != nil {
		log.Printf("initialize logger failed: %v", err)
		return
	}
	defer func() { _ = logger.Sync() }()

	gin.SetMode(cfg.Server.Mode)

	if err := database.InitMySQL(cfg); err != nil {
		logger.Error("mysql_init_failed", zap.Error(err))
		return
	}
	logger.Info("mysql_connected")

	if err := database.InitRedis(cfg); err != nil {
		logger.Error("redis_init_failed", zap.Error(err))
		return
	}
	logger.Info("redis_connected", zap.String("addr", cfg.Redis.Addr), zap.Int("db", cfg.Redis.DB))

	if err := database.AutoMigrate(); err != nil {
		logger.Error("auto_migrate_failed", zap.Error(err))
		return
	}
	logger.Info("auto_migrate_completed")

	reportCache := reportcache.NewReportCache(
		database.RDB,
		time.Duration(cfg.ReportCache.TTLSeconds)*time.Second,
		cfg.ReportCache.Enabled,
	)
	logger.Info("report_cache_initialized",
		zap.Bool("enabled", cfg.ReportCache.Enabled),
		zap.Int("ttl_seconds", cfg.ReportCache.TTLSeconds),
	)

	r := router.SetupRouter(cfg, reportCache, logger)
	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	if err := r.Run(addr); err != nil {
		logger.Error("server_stopped", zap.Error(err))
	}
}
