package main

import (
	"errors"
	"flash-sale-system/internal/auth"
	"flash-sale-system/internal/cache"
	"flash-sale-system/internal/config"
	"flash-sale-system/internal/database"
	"flash-sale-system/internal/logger"
	"flash-sale-system/internal/middleware"
	"flash-sale-system/internal/model"
	"flash-sale-system/internal/mq"
	"flash-sale-system/internal/service"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {

	logger.InitLogger()     //启动日志系统，调用InitLogger函数初始化全局日志对象
	defer logger.Log.Sync() // zap有缓冲区，确保日志在程序退出时被同步

	err := config.InitConfig() //加载配置文件，调用InitConfig函数从config.yaml加载配置到全局变量GlobalConfig中
	if err != nil {
		panic(err)
	}

	err = database.InitMySQL() //连接MySQL数据库，调用InitMySQL函数使用gorm连接MySQL，并将连接对象保存在database.DB中
	if err != nil {
		panic(err)
	}

	err = database.DB.AutoMigrate(&model.Order{}) //自动创建/更新orders表
	if err != nil {
		panic(err)
	}

	cfg := config.GlobalConfig //从全局配置变量中获取配置项，方便后续使用

	//连接Redis，创建Redis客户端对象rdb，使用配置文件中的Redis连接信息
	rdb := cache.NewRedisClient(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)

	//连接RabbitMQ，创建RabbitMQ客户端对象rabbitMQ，创建order_queue队列
	rabbitMQ, err := mq.NewRabbitMQ(
		"amqp://guest:guest@localhost:5672/", //RabbitMQ连接URL，格式为amqp://用户名:密码@主机:端口/
	)
	if err != nil {
		panic(err)
	}

	defer rabbitMQ.Conn.Close()
	defer rabbitMQ.Channel.Close() //程序退出时关闭RabbitMQ连接和通道，释放资源

	stockService := service.NewStockService(rdb) //创建库存服务对象，传入Redis客户端对象，库存服务负责处理库存相关的逻辑，例如扣减库存和回滚库存
	orderService := service.NewOrderService()    //创建订单服务对象，订单服务负责处理订单相关的逻辑，例如创建订单和查询订单状态

	//启动RabbitMQ消费者，监听order_queue队列，处理订单消息
	err = rabbitMQ.StartConsumer(orderService)
	if err != nil {
		panic(err)
	}

	r := gin.Default() //创建Gin引擎对象，使用默认的中间件（日志和恢复），用于处理HTTP请求和路由

	r.Use(middleware.RateLimitMiddleware(rdb, 10)) //全局使用限流中间件，限制每个IP每秒最多只能发起10次请求，防止恶意攻击和过载

	//初始化商品库存，设置初始库存数量为1000，过期时间为0表示永不过期，SETNX命令确保只有在键不存在时才设置值，避免重复初始化导致库存数量错误
	err = rdb.Client.SetNX(
		cache.Ctx,         //全局上下文对象，go-redis库的操作需要一个上下文参数，这里传入cache.Ctx
		"stock:product:1", //库存键，格式为stock:product:{id}，这里假设只有一个商品，ID为1
		1000,              //初始库存数量，设置为1000
		0,                 //过期时间，0表示永不过期
	).Err()

	if err != nil {
		panic(err)
	} //初始化商品库存，设置初始库存数量为5，过期时间为0表示永不过期

	//登录接口，生成JWT token
	r.POST("/login", func(c *gin.Context) {

		//定义一个请求体结构体，包含一个user_id字段，用于接收登录请求中的用户ID参数
		var req struct {
			UserID string `json:"user_id"`
		}

		//将请求体中的 JSON 数据绑定到 req 结构体对象中，如果绑定失败或者 user_id 为空，返回 400 Bad Request 错误响应
		err := c.ShouldBindJSON(&req)
		if err != nil || req.UserID == "" {
			c.JSON(400, gin.H{
				"error": "invalid user_id",
			})
			return
		}

		//生成JWT token，调用auth.GenerateToken函数生成一个包含用户ID的JWT token，如果生成失败，返回 500 Internal Server Error 错误响应
		token, err := auth.GenerateToken(req.UserID)
		if err != nil {
			c.JSON(500, gin.H{
				"error": "generate token failed",
			})
			return
		}

		//返回生成的JWT token，使用 200 OK 状态码和 JSON 格式的响应体，包含一个 token 字段，值为生成的JWT token
		c.JSON(200, gin.H{
			"token": token,
		})
	})

	authGroup := r.Group("/")            //创建一个受保护的路由组，所有在这个组中的路由都需要经过JWT认证中间件的验证才能访问
	authGroup.Use(auth.AuthMiddleware()) //受保护的路由组，使用JWT认证中间件，只有携带有效JWT token的请求才能访问这些路由

	//秒杀接口，用户可以访问这个接口尝试秒杀商品，接口会验证JWT token，扣减库存，并发送消息到RabbitMQ异步创建订单
	authGroup.GET("/seckill", func(c *gin.Context) {

		//从上下文中获取用户ID，JWT认证中间件会将用户ID存储在上下文中，这里通过c.Get("userID")获取用户ID，如果不存在则返回 401 Unauthorized 错误响应
		userIDVal, exists := c.Get("userID")
		if !exists {
			c.JSON(500, gin.H{
				"error": "no user",
			})
			return
		}

		userID := userIDVal.(string)  //从JWT中拿到用户ID，进行类型断言
		productID := "1"              //假设只有一个商品，ID为1，实际应用中可以从请求参数中获取商品ID
		stockKey := "stock:product:1" //库存键，格式为stock:product:{id}，这里假设只有一个商品，ID为1

		//扣减库存，调用stockService.DeductStock函数尝试扣减库存
		result, err := stockService.DeductStock(stockKey)
		//如果扣减库存失败，记录错误日志并返回 500 Internal Server Error 错误响应
		if err != nil {

			logger.Log.Error("deduct stock failed",
				zap.Error(err),
				zap.String("user_id", userID),
				zap.String("product_id", productID),
			)

			c.JSON(500, gin.H{
				"error": "system error",
			})
			return
		}

		//如果扣减库存的结果为0，说明库存已经售罄，记录警告日志并返回 200 OK 状态码和 JSON 格式的响应体，包含一个 msg 字段，值为 "sold out"
		if result == 0 {

			logger.Log.Warn("stock sold out",
				zap.String("product_id", productID),
				zap.String("user_id", userID),
			)

			c.JSON(200, gin.H{"msg": "sold out"})
			return
		}

		//扣减库存成功，发送消息到RabbitMQ，异步创建订单
		err = rabbitMQ.Publish(
			mq.OrderMessage{
				UserID:    userID,
				ProductID: productID,
			},
		)

		//如果发送消息失败，尝试回滚库存，记录日志，但不影响用户体验，继续返回订单创建失败的响应
		if err != nil {

			rollbackErr := stockService.RollbackStock(stockKey)

			//如果回滚库存失败，记录错误日志，包含用户ID，帮助排查问题，但不影响用户体验，继续返回订单创建失败的响应
			if rollbackErr != nil {

				logger.Log.Error("rollback stock failed",
					zap.Error(rollbackErr),
					zap.String("user_id", userID),
				)

			}

			//记录发送消息失败的错误日志
			logger.Log.Error(
				"publish mq failed",
				zap.Error(err),
			)

			c.JSON(500, gin.H{
				"error": "mq error",
			})
			return
		}

		//记录成功接受秒杀请求的日志，包含用户ID和商品ID
		logger.Log.Info(
			"seckill request accepted",
			zap.String("user_id", userID),
			zap.String("product_id", productID),
		)

		c.JSON(200, gin.H{
			"msg": "queued",
		})
	})

	//订单查询接口，用户可以查询自己的订单状态
	authGroup.GET("/order/check", func(c *gin.Context) {

		//从上下文中获取用户ID，JWT认证中间件会将用户ID存储在上下文中，这里通过c.Get("userID")获取用户ID，如果不存在则返回 401 Unauthorized 错误响应
		userIDVal, exists := c.Get("userID")
		if !exists {
			c.JSON(401, gin.H{
				"error": "no user",
			})
			return
		}

		userID := userIDVal.(string) //从JWT中拿到用户ID，进行类型断言

		var order model.Order //定义一个订单对象，用于保存查询到的订单信息

		//查询数据库，查找用户的订单信息，如果查询失败则返回 404 Not Found 错误响应，说明没有找到订单
		err := database.DB.
			Where("user_id=?", userID).
			First(&order).Error

		if err != nil {
			c.JSON(404, gin.H{
				"msg": "order not found",
			})
			return
		}

		c.JSON(200, order) //返回查询到的订单信息，使用 200 OK 状态码和 JSON 格式的响应体，包含订单对象的所有字段
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port) //构建服务器地址字符串，例如":8080"

	//记录服务器启动的日志，包含服务器地址信息，使用Info级别的日志，方便监控服务器的运行状态和访问地址
	logger.Log.Info(
		"server started",
		zap.String("addr", addr),
	)

	//启动HTTP服务器，监听指定端口，处理请求
	err = r.Run(addr)
	//如果服务器启动失败，记录错误日志并退出程序
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Log.Fatal(
			"server start failed",
			zap.Error(err),
		)
	}
}
