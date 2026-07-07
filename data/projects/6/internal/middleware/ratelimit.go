package middleware

import (
	"flash-sale-system/internal/cache"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitMiddleware 实现一个简单的基于 Redis 的限流中间件，限制每个接口每秒钟的请求次数，防止恶意攻击和过载
func RateLimitMiddleware(rdb *cache.RedisClient, limit int) gin.HandlerFunc { 

	//返回一个 gin.HandlerFunc 类型的函数，这个函数会在每个请求到达时被调用，执行限流逻辑
	return func(c *gin.Context) {

		key := "rate_limit:" + c.FullPath() //每个接口独立限流key

		//使用 Redis 的 INCR 命令增加请求计数器，如果计数器不存在则会被创建并初始化为 1，返回当前计数值
		count, err := rdb.Client.Incr(cache.Ctx, key).Result() 

		//如果 Redis 操作失败，返回 500 Internal Server Error 错误，并中止请求处理
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "redis error",
			})
			c.Abort()  //中止请求处理，返回错误响应
			return
		}

		//如果计数器的值为 1，说明这是第一次请求，需要设置过期时间为 1 秒，确保计数器在 1 秒后自动重置
		if count == 1 { 
			err := rdb.Client.Expire(cache.Ctx, key, time.Second).Err()

			//如果设置过期时间失败，返回 500 Internal Server Error 错误，并中止请求处理
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "expire error",
				})
				c.Abort()  //中止请求处理，返回错误响应
				return
			}
		}

		//如果请求数超过限制，返回 429 Too Many Requests 错误，并中止请求处理
		if count > int64(limit) { 
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests",
			})
			c.Abort()  //中止请求处理，返回错误响应
			return
		}

		c.Next() //放行请求，继续处理后续的 handler
	}
}
