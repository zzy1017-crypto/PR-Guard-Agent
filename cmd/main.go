package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/router"
)

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	gin.SetMode(cfg.Server.Mode)

	r := router.SetupRouter()
	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	if err := r.Run(addr); err != nil {
		log.Fatalf("start server failed: %v", err)
	}
}
