package service

import (
	"context"
	"flash-sale-system/internal/cache"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// StockService 提供库存管理的服务，使用 Redis 作为库存数据的存储和操作工具
type StockService struct {
	client *redis.Client
}

// NewStockService 创建一个新的 StockService 实例，接受一个 RedisClient 对象作为参数，返回一个 StockService 对象
func NewStockService(rdb *cache.RedisClient) *StockService {
	return &StockService{
		client: rdb.Client,
	}
}

// 定义一个 Lua 脚本，用于原子地扣减库存，确保在高并发环境下不会出现竞争条件
var deductStockScript = redis.NewScript(`
local stock = tonumber(redis.call("get", KEYS[1]))     --获取当前库存数量，并转换为数字类型,local定义局部变量,tonumber将字符串转为数字
if stock == nil then
   return -1
end                                                --如果库存不存在，返回 -1 表示错误

if stock <= 0 then
return  0
end                                                 --如果库存不足，返回 0 表示扣减失败

redis.call("decr", KEYS[1])            --库存足够，执行扣减操作，使用 Redis 的 DECR 命令将库存数量减 1,decr命令会自动将字符串类型的库存数量转换为数字类型,并进行原子操作,确保在高并发环境下不会出现竞争条件
return 1                                    --扣减成功，返回 1 表示成功  
`)

// DeductStock 使用 Lua 脚本原子地扣减库存，接受一个库存键作为参数，返回扣减结果和错误信息
func (s *StockService) DeductStock(key string) (int64, error) {
	res, err := deductStockScript.Run(
		context.Background(),   //Lua 脚本的执行需要一个上下文参数，这里使用 context.Background() 创建一个背景上下文，表示没有特定的取消或超时机制
		s.client,  //Redis 客户端对象，用于执行 Lua 脚本，传递给 Run 方法以便在 Redis 服务器上执行脚本
		[]string{key},  //Lua 脚本的键参数，这里传递一个包含库存键的字符串切片，Lua 脚本中通过 KEYS[1] 来访问这个键
	).Result()  

	if err != nil {
		return 0, err
	}

	//将 Lua 脚本的执行结果转换为 int64 类型，表示扣减结果，如果转换失败则返回错误
	result, ok := res.(int64)
	if !ok {
		return 0, fmt.Errorf("invalid result type: %T", res)
	} 

	return result, nil
}

// RollbackStock 回滚库存，接受一个库存键作为参数，返回错误信息
func (s *StockService) RollbackStock(key string) error {

	//回滚库存，使用 Redis 的 INCR 命令将库存数量加 1，确保在订单处理失败时能够恢复库存数量
	return s.client.Incr(
		context.Background(),  
		key,
	).Err()
}
