package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/router"
	reportcache "pr-guard-agent/pkg/cache"
)

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	gin.SetMode(cfg.Server.Mode)

	if err := database.InitMySQL(cfg); err != nil {
		log.Fatalf("init mysql failed: %v", err)
	}

	if err := database.InitRedis(cfg); err != nil {
		log.Fatalf("init redis failed: %v", err)
	}

	if err := database.AutoMigrate(); err != nil {
		log.Fatalf("auto migrate failed: %v", err)
	}

	reportCache := reportcache.NewReportCache(
		database.RDB,
		time.Duration(cfg.ReportCache.TTLSeconds)*time.Second,
		cfg.ReportCache.Enabled,
	)
	log.Printf(
		"report cache initialized: enabled=%t ttl_seconds=%d redis_addr=%s redis_db=%d",
		cfg.ReportCache.Enabled,
		cfg.ReportCache.TTLSeconds,
		cfg.Redis.Addr,
		cfg.Redis.DB,
	)

	r := router.SetupRouter(cfg, reportCache)
	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	if err := r.Run(addr); err != nil {
		log.Fatalf("start server failed: %v", err)
	}
}
