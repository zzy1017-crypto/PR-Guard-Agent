package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var Ctx = context.Background()   //全局上下文对象，go-redis库的操作需要一个上下文参数，这里创建一个全局的背景上下文，可以在程序中其他地方使用cache.Ctx来传递这个上下文

// RedisClient 包装了 go-redis 的客户端对象，提供了一个结构体来封装 Redis 客户端，方便在程序中使用和扩展
type RedisClient struct {
	Client *redis.Client
} 

// NewRedisClient 创建一个新的 Redis 客户端，接受 Redis 的连接信息作为参数，返回一个 RedisClient 对象
func NewRedisClient(addr, password string, db int) *RedisClient {
	rdb := redis.NewClient(&redis.Options{ 
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	//测试 Redis 连接是否成功，使用 Ping 命令，如果连接失败，panic 并输出错误信息
	_, err := rdb.Ping(Ctx).Result() 
	if err != nil {
		panic("Redis 连接失败:" + err.Error())
	}

	//返回一个 RedisClient 对象，包含了创建的 Redis 客户端，可以在程序中其他地方使用这个对象来操作 Redis
	return &RedisClient{
		Client: rdb,
	}
}
