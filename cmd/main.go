package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
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

	r, workerManager := router.SetupRouterWithWorker(cfg, reportCache, logger)
	appCtx := context.Background()
	if err := workerManager.Start(appCtx); err != nil {
		logger.Error("analysis_worker_start_failed", zap.Error(err))
		return
	}
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{Addr: addr, Handler: r}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		logger.Info("shutdown_signal_received")
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_stopped", zap.Error(err))
		}
	}

	workerWait := time.Duration(cfg.AnalysisWorker.TaskTimeoutSeconds+5) * time.Second
	workerShutdownCtx, cancelWorkers := context.WithTimeout(context.Background(), workerWait)
	workerShutdownDone := make(chan error, 1)
	go func() {
		workerShutdownDone <- workerManager.Shutdown(workerShutdownCtx)
	}()

	httpShutdownCtx, cancelHTTP := context.WithTimeout(context.Background(), 10*time.Second)
	if err := server.Shutdown(httpShutdownCtx); err != nil {
		logger.Warn("http_server_shutdown_failed", zap.Error(err))
	}
	cancelHTTP()

	if err := <-workerShutdownDone; err != nil {
		logger.Warn("analysis_worker_shutdown_failed", zap.Error(err))
	}
	cancelWorkers()
}
