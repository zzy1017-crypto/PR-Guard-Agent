package mq

import (
	"encoding/json"
	"flash-sale-system/internal/service"
)


func (mq *RabbitMQ) StartConsumer(
	orderService *service.OrderService,
) error {
	//开始监听 "order_queue" 队列，获取消息通道 msgs
	msgs, err := mq.Channel.Consume(
		"order_queue",
		"",
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		return err
	}

	//启动协程持续消费
	go func() {
		//从消息通道 msgs 中不断接收消息，处理每条订单消息
		for msg := range msgs {

			var orderMsg OrderMessage  //定义一个订单消息对象，用于保存从消息中解析出的订单信息，包含用户ID和商品ID等字段

			//将消息体中的 JSON 数据反序列化到订单消息对象中
			err := json.Unmarshal(
				msg.Body,
				&orderMsg,
			) 

			//如果反序列化失败则跳过当前消息，继续处理下一条消息，确保消费者的稳定性和健壮性
			if err != nil {
				continue
			}
			
			//调用订单服务的 CreateOrder 方法创建订单，传入消息中的用户ID和商品ID，忽略返回的订单对象和错误
			_, _ = orderService.CreateOrder(
				orderMsg.UserID,
				orderMsg.ProductID,
			)
		}
	}()

	return nil
}
