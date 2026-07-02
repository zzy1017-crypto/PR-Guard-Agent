package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisLock struct {
	client *redis.Client
} // RedisLock 结构体封装了一个 Redis 客户端，用于实现分布式锁的功能

func NewRedisLock(client *redis.Client) *RedisLock {
	return &RedisLock{
		client: client,
	}
} //依赖注入，将 Redis 客户端传入 RedisLock 结构体中，使其能够使用 Redis 的功能来实现锁的操作

func (l *RedisLock) Lock(key string, value string, expiration time.Duration) (bool, error) {
	return l.client.SetNX(context.Background(), key, value, expiration).Result()
}

var unlockScript = redis.NewScript(`
if redis.call("get",KEYS[1]) == ARGV[1] then  //如果锁的值与传入的值相同
    return redis.call("del",KEYS[1])  //允许解锁，删除锁
else
	return 0
end
`) //Redis Lua 脚本，原子操作，用于安全地解锁，确保只有持有锁的客户端才能解锁

func (l *RedisLock) Unlock(key, value string) error {
	_, err := unlockScript.Run(
		context.Background(),
		l.client,
		[]string{key}, //KEYS[1] 是锁的键
		value,         //ARGV[1] 是锁的值
	).Result()
	return err

}
