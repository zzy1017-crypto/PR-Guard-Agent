package service

import "errors"

type OrderService struct{}

type OrderRepository interface {
	CreateOrder(userID int64) error
}

const DefaultStock = 100

var ErrDuplicateOrder = errors.New("duplicate order")

func NewOrderService() *OrderService {
	return &OrderService{}
}

func (s *OrderService) CreateOrder(userID int64) error {
	if userID <= 0 {
		return ErrDuplicateOrder
	}
	return nil
}
