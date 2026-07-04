package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/router"
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

	r := router.SetupRouter(cfg)
	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	if err := r.Run(addr); err != nil {
		log.Fatalf("start server failed: %v", err)
	}
}
