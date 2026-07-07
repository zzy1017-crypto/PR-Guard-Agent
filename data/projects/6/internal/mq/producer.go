package mq

import (
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

// OrderMessage 定义了秒杀成功后的订单消息的结构，包含用户ID和商品ID
type OrderMessage struct {
	UserID    string
	ProductID string
}

// Publish 方法将订单消息发布到 RabbitMQ 的 "order_queue" 队列中，消息内容以 JSON 格式编码
func (mq *RabbitMQ) Publish(msg OrderMessage) error {

	body, err := json.Marshal(msg) //把结构体转JSON
	if err != nil {
		return err
	}
	//发送消息
	return mq.Channel.Publish(
		"",            //交换机名称，空字符串表示使用默认交换机
		"order_queue", //路由键，指定消息发送到哪个队列，这里是 "order_queue"
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json", //消息内容类型，指定为 JSON
			Body:        body,               //消息体，包含订单信息的 JSON 数据
		},
	)
}
