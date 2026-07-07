package mq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQ struct {
	Conn    *amqp.Connection //与RabbitMQ服务器的TCP连接
	Channel *amqp.Channel    //在连接上创建的通道，用于发送和接收消息
}

// 创建RabbitMQ实例，连接到RabbitMQ服务器，并声明一个队列
func NewRabbitMQ(url string) (*RabbitMQ, error) {

	//连接到RabbitMQ服务器，返回一个连接对象，如果连接失败则返回错误
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	} 

	//在连接上创建一个通道，返回一个通道对象，如果创建通道失败则返回错误
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	} 

	//声明一个队列，确保队列存在，如果声明失败则返回错误
	_, err = ch.QueueDeclare(
		"order_queue", //队列名称
		true,          //持久化，确保RabbitMQ重启后队列仍然存在
		false,         //自动删除，当没有消费者时删除队列
		false,         //独占，限制队列只能被当前连接使用
		false,         //no-wait，异步声明队列，不等待服务器响应
		nil,           //其他参数
	)

	if err != nil {
		return nil, err
	}

	//返回一个RabbitMQ对象，包含了连接和通道，可以在程序中其他地方使用这个对象来发送和接收消息
	return &RabbitMQ{
		Conn:    conn,
		Channel: ch,
	}, nil

}
