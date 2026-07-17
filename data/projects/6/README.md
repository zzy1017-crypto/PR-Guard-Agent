# Flash Sale System

基于 Go + Gin + Redis + Lua + RabbitMQ + MySQL 实现的秒杀系统示例项目。

项目模拟电商秒杀场景，重点解决高并发请求下的库存扣减、超卖控制、重复下单、接口限流和异步订单创建问题。整体链路覆盖登录鉴权、秒杀请求、Redis 原子扣库存、消息队列削峰、MySQL 订单落库和压测验证，适合作为 Go 后端实习项目展示。

## 核心能力

- 秒杀接口：支持用户携带 JWT Token 发起秒杀请求。
- 原子扣库存：使用 Redis Lua 脚本保证库存判断和扣减的原子性。
- 异步下单：使用 RabbitMQ 将订单创建从主请求链路中解耦。
- 幂等控制：使用 MySQL 联合唯一索引防止同一用户重复下单。
- 接口限流：使用 Redis 实现固定窗口限流，防止接口被恶意刷请求。
- 压测验证：提供 Go 并发压测脚本，已完成 1000 用户、100 并发场景验证。

## 项目亮点

- 使用 Redis + Lua 将库存读取、库存判断、库存扣减合并为一次原子操作，避免高并发场景下的库存超卖。
- 使用 RabbitMQ 异步处理订单创建，将秒杀接口和数据库写入解耦，降低 MySQL 瞬时写入压力。
- 使用 MySQL 事务和 `user_id + product_id` 联合唯一索引保证同一用户对同一商品不会重复下单。
- 使用 Redis `INCR + EXPIRE` 实现固定窗口限流中间件，限制单位时间内的接口请求量。
- 使用 JWT 完成登录认证，受保护接口通过中间件解析 Token 并获取用户身份。
- 使用 Viper 管理 YAML 配置，使用 Zap 输出结构化日志。
- 使用 Docker Compose 编排 Redis、RabbitMQ、MySQL，便于本地开发和依赖启动。
- 提供 Go 并发压测脚本，可模拟多用户并发登录和秒杀请求。
- 关闭限流后完成 1000 用户、100 并发压测：成功 1000 次，失败 0 次，约 2364 QPS，库存从 1000 扣减至 0，MySQL 生成 1000 条订单且无重复订单。

## 技术栈

| 类型 | 技术 |
| --- | --- |
| 编程语言 | Go |
| Web 框架 | Gin |
| 缓存/库存 | Redis、go-redis、Lua Script |
| 数据库 | MySQL、GORM |
| 消息队列 | RabbitMQ、amqp091-go |
| 鉴权 | JWT |
| 配置管理 | Viper、YAML |
| 日志 | Zap |
| 部署 | Docker、Docker Compose |
| 压测 | Goroutine、WaitGroup、atomic、net/http |

## 核心流程

```text
用户登录
  |
  v
获取 JWT Token
  |
  v
请求 /seckill
  |
  v
JWT 鉴权中间件
  |
  v
Redis 限流中间件
  |
  v
Redis Lua 原子扣减库存
  |
  v
投递订单消息到 RabbitMQ
  |
  v
消费者异步创建订单
  |
  v
MySQL 持久化订单
```

## 项目结构

```text
flash-sale-system
├── cmd
│   └── main.go                 # 程序入口，路由注册和服务启动
├── internal
│   ├── auth                    # JWT 生成、解析和认证中间件
│   ├── cache                   # Redis 客户端和 Redis 分布式锁封装
│   ├── config                  # Viper 配置加载
│   ├── database                # MySQL / GORM 初始化
│   ├── logger                  # Zap 日志初始化
│   ├── middleware              # Redis 限流中间件
│   ├── model                   # 订单模型
│   ├── mq                      # RabbitMQ 连接、生产者、消费者
│   └── service                 # 库存扣减和订单创建业务逻辑
├── scripts
│   └── pressure.go             # 并发压测脚本
├── config.yaml                 # 服务配置
├── docker-compose.yml          # Redis / RabbitMQ / MySQL 编排
├── Dockerfile                  # 应用镜像构建文件
├── go.mod
└── README.md
```

## 环境要求

- Go 1.25+
- Docker Desktop / Docker Engine
- MySQL 客户端可选，用于手动查询订单
- Redis 客户端可选，用于手动查询库存

## 配置说明

当前 `config.yaml` 示例：

```yaml
server:
  port: 8080

mysql:
  host: localhost
  port: 3307
  user: root
password: <MYSQL_PASSWORD>
  dbname: flash_sale

redis:
  addr: localhost:6380
  password: ""
  db: 0

rabbitmq:
  url: amqp://guest:guest@localhost:5672/
```

Redis 在 `docker-compose.yml` 中映射为：

```yaml
ports:
  - "6380:6379"
```

这样可以避免和本机或 WSL 中已有的 `6379` Redis 端口冲突。查询库存时也应使用 `6380`：

```bash
redis-cli -h 127.0.0.1 -p 6380 GET stock:product:1
```

## 快速启动

### 1. 启动依赖服务

```bash
docker compose up -d
```

启动后会包含：

- Redis：`127.0.0.1:6380`
- RabbitMQ：`127.0.0.1:5672`
- RabbitMQ Management：`http://localhost:15672`
- MySQL：`127.0.0.1:3307`

RabbitMQ 默认账号密码：

```text
guest / guest
```

### 2. 启动 Go 服务

```bash
go run ./cmd
```

服务启动后默认监听：

```text
http://localhost:8080
```

### 3. 登录获取 Token

```bash
curl -X POST "http://localhost:8080/login" \
  -H "Content-Type: application/json" \
  -d "{\"user_id\":\"1\"}"
```

响应示例：

```json
{
  "token": "your_jwt_token"
}
```

### 4. 请求秒杀接口

```bash
curl "http://localhost:8080/seckill" \
  -H "Authorization: Bearer your_jwt_token"
```

响应示例：

```json
{
  "msg": "queued"
}
```

含义说明：

- `queued`：库存扣减成功，订单消息已进入 RabbitMQ，等待异步落库。
- `sold out`：库存不足。
- `too many requests`：触发 Redis 限流。

### 5. 查询订单

```bash
curl "http://localhost:8080/order/check" \
  -H "Authorization: Bearer your_jwt_token"
```

## 库存与订单校验

查询 Redis 库存：

```bash
redis-cli -h 127.0.0.1 -p 6380 GET stock:product:1
```

如果没有本机 `redis-cli`，也可以进入 Docker 容器查询：

```bash
docker exec -it flash-sale-redis redis-cli GET stock:product:1
```

查询 MySQL 订单数量：

```bash
mysql -h 127.0.0.1 -P 3307 -u root -p
```

进入 MySQL 后执行：

```sql
USE flash_sale;
SELECT COUNT(*) FROM orders;
SELECT user_id, product_id, COUNT(*) AS cnt
FROM orders
GROUP BY user_id, product_id
HAVING cnt > 1;
```

第二条 SQL 如果没有结果，说明没有重复订单。

## 压测

项目提供了压测脚本：

```bash
go run ./scripts/pressure.go
```

默认压测配置：

```go
const (
    baseURL     = "http://localhost:8080"
    totalUsers  = 1000
    concurrency = 100
)
```

脚本会模拟 1000 个用户，在 100 并发下执行：

```text
登录获取 Token -> 携带 Token 请求秒杀接口
```

### 关闭限流后的压测结果

在关闭 Redis 限流后，使用默认压测配置进行测试：

```text
========== Pressure Test Result ==========
Total users: 1000
Concurrency: 100
Success: 1000
Failed: 0
Total time: 423ms
Approx QPS: 2364
```

压测后数据校验：

```text
Redis 库存：1000 -> 0
MySQL 订单数：1000
重复订单数：0
RabbitMQ 队列积压：0
```

该结果验证了在高并发请求下：

- Redis Lua 原子扣库存后没有出现超卖。
- MySQL 订单数量与成功秒杀请求数量一致。
- `user_id + product_id` 联合唯一索引保证没有重复订单。
- RabbitMQ 消费完成后没有消息积压。

### 开启限流时的说明

当前主程序中默认启用了 Redis 限流：

```go
r.Use(middleware.RateLimitMiddleware(rdb, 10))
```

如果保持该限流配置进行压测，大量请求会被拦截并返回 `429 Too Many Requests`。这种情况下成功数较少是预期行为，并不代表库存扣减或订单创建链路异常。

压测时建议重点校验：

- Redis 库存是否按成功请求数扣减。
- MySQL 订单数是否等于成功进入队列并消费成功的请求数。
- 是否存在重复订单。
- RabbitMQ 是否存在消息积压。

## 核心实现说明

### 1. Redis Lua 原子扣库存

库存扣减逻辑位于 `internal/service/stock.go`。

Lua 脚本完成以下操作：

```text
读取库存 -> 判断库存是否存在 -> 判断库存是否大于 0 -> 执行 DECR -> 返回结果
```

由于 Lua 脚本在 Redis 中原子执行，可以避免多个请求同时读取同一个库存值后重复扣减的问题。

### 2. RabbitMQ 异步下单

秒杀接口扣减库存成功后，会将订单消息投递到 `order_queue`。

消费者监听该队列，并调用订单服务创建订单：

```text
Publish OrderMessage -> Consume order_queue -> CreateOrder -> MySQL
```

这种方式可以将接口响应和数据库写入解耦，缓解高并发请求直接打到 MySQL 的压力。

### 3. 订单幂等

订单模型中使用了 `user_id + product_id` 联合唯一索引，订单创建时也会在事务中先检查是否已经存在相同订单。

这可以防止同一个用户对同一个商品重复下单。

### 4. Redis 限流

限流中间件位于 `internal/middleware/ratelimit.go`。

实现方式：

```text
按接口路径生成限流 key
Redis INCR 计数
首次请求设置 1 秒过期时间
超过阈值返回 429 Too Many Requests
```

当前主程序中全局配置为：

```go
r.Use(middleware.RateLimitMiddleware(rdb, 10))
```

即每个接口路径每秒最多允许 10 次请求。

### 5. JWT 鉴权

登录接口 `/login` 根据 `user_id` 生成 JWT。

秒杀接口和订单查询接口位于受保护路由组中，请求时必须携带：

```text
Authorization: Bearer <token>
```

## 常见问题

### 1. 为什么订单创建了，但 Redis 库存看起来还是 1000？

优先确认查询的是不是项目实际连接的 Redis。

当前项目配置使用：

```yaml
redis:
  addr: localhost:6380
```

因此推荐查询：

```bash
redis-cli -h 127.0.0.1 -p 6380 GET stock:product:1
```

如果误查了本机 `6379` 或其他 WSL Redis，可能会看到不一致的数据。

### 2. 为什么压测成功数很少？

因为项目启用了全局限流：

```go
r.Use(middleware.RateLimitMiddleware(rdb, 10))
```

默认每秒只允许 10 次请求通过，压测时大量请求会被限流拦截。这是预期行为。

如果需要验证秒杀主链路的最大处理能力，可以临时关闭或调高限流阈值。关闭限流后，本项目在 1000 用户、100 并发场景下压测成功 1000 次，失败 0 次，Redis 库存从 1000 扣减至 0，MySQL 生成 1000 条订单且未发现重复订单。

### 3. 为什么重启服务后库存没有重新变成 1000？

当前库存初始化使用 `SetNX`，只有当 `stock:product:1` 不存在时才会写入初始库存。这样可以避免服务重启覆盖已有库存。

如果需要重新初始化库存，可以手动执行：

```bash
redis-cli -h 127.0.0.1 -p 6380 SET stock:product:1 1000
```

## 简历描述参考

可以在简历中这样描述该项目：

> 基于 Go + Gin 实现高并发秒杀系统，使用 Redis + Lua 实现库存原子扣减，避免并发场景下库存超卖；引入 RabbitMQ 异步创建订单，实现接口请求与数据库写入解耦；使用 MySQL 事务和联合唯一索引保证订单幂等；基于 Redis 实现接口限流，并结合 JWT 鉴权、Zap 日志、Viper 配置和 Docker Compose 完成服务工程化。

也可以补充压测验证：

> 编写 Go 压测脚本模拟 1000 用户、100 并发请求；关闭限流后压测耗时约 423ms，成功 1000 次、失败 0 次，约 2364 QPS；压测后 Redis 库存由 1000 扣减至 0，MySQL 生成 1000 条订单，且 SQL 分组校验未发现重复订单。

## 后续优化方向

- RabbitMQ 消费端改为手动 ACK，提升消息可靠性。
- RabbitMQ 生产端增加 Confirm 机制，确认消息是否成功到达 Broker。
- MySQL、RabbitMQ 连接信息统一从 `config.yaml` 读取，减少硬编码。
- 增加订单状态流转，例如 `created`、`paid`、`cancelled`。
- 增加库存预热接口和后台管理接口。
- 增加单元测试、集成测试和接口测试。
- 引入 Prometheus + Grafana 做监控。
- 支持 Docker Compose 一键启动完整应用服务。
