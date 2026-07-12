package database

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"pr-guard-agent/internal/config"
)

var RDB *redis.Client // Redis客户端实例

func InitRedis(cfg *config.Config) error {
	redisCfg := cfg.Redis

	//创建一个新的Redis客户端实例，使用配置文件中读取的Redis连接信息，包括地址、密码和数据库索引
	client := redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})

	//使用context.WithTimeout创建一个带有超时的上下文，设置超时时间为5秒，避免外部依赖卡死
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//使用Ping方法测试与Redis服务器的连接，如果连接失败，返回错误信息
	if err := client.Ping(ctx).Err(); err != nil {
		return err
	}

	RDB = client

	return nil
}
