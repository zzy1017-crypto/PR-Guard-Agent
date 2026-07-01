package service

import (
	"errors"
	"flash-sale-system/internal/database"
	"flash-sale-system/internal/model"
	"fmt"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var ErrDuplicateOrder = errors.New("duplicate order") //定义一个全局错误变量，表示重复订单错误

type OrderService struct{}  //订单服务对象，负责处理订单相关的业务逻辑，例如创建订单、查询订单等

// NewOrderService 创建一个新的 OrderService 实例，返回一个 OrderService 对象
func NewOrderService() *OrderService {
	return &OrderService{}
}

// CreateOrder 创建订单，接受用户ID和商品ID作为参数，返回订单对象和错误信息
func (s *OrderService) CreateOrder(userID, productID string) (*model.Order, error) {
	var order *model.Order //定义一个订单对象，用于保存创建的订单信息

	//使用数据库事务来确保订单创建过程的原子性，避免在高并发环境下出现数据不一致的问题
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var count int64  //定义一个变量count，用于统计同一用户对同一商品的订单数量，确保幂等性检查的准确性

		//查询数据库，统计同一用户对同一商品的订单数量，如果查询失败则返回错误
		err := tx.Model(&model.Order{}).
			Where("user_id = ? AND product_id = ?", userID, productID). //联合索引查询条件，确保查询效率
			Count(&count).Error //执行查询操作，将结果保存在count变量中，如果查询失败则返回错误

		if err != nil {
			return err
		}

		//如果订单数量大于0，说明用户已经对该商品下过订单，返回重复订单错误，确保幂等性
		if count > 0 {
			return ErrDuplicateOrder
		} 

		//创建订单对象，包含订单ID、用户ID、商品ID、订单状态和创建时间等信息，订单ID使用当前时间戳生成，确保唯一性
		order := &model.Order{
			OrderID:   fmt.Sprintf("order_%d", time.Now().UnixNano()),
			UserID:    userID,
			ProductID: productID,
			Status:    "created",
			CreatedAt: time.Now(),
		}

		//将订单对象保存到数据库中，如果保存失败则返回错误，如果是重复订单错误则返回特定的错误信息，确保幂等性检查的准确性
		err = tx.Create(order).Error
		if err != nil {
			//检查错误是否是重复订单错误，如果是则返回特定的错误信息，确保幂等性检查的准确性
			if isDupilicateEntryError(err) {
				return ErrDuplicateOrder
			}
			return err
		}

		//将创建的订单对象保存在外部定义的订单变量中，以便在事务函数外部返回订单信息
		return nil

	})

	//如果事务执行失败则返回错误
	if err != nil {
		return nil, err
	}

	//返回创建的订单对象和nil错误，表示订单创建成功
	return order, nil

}

// isDupilicateEntryError 检查错误是否是 MySQL 的重复条目错误，接受一个错误对象作为参数，返回一个布尔值表示是否是重复条目错误
func isDupilicateEntryError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError  //定义一个 MySQL 错误对象，用于检查错误类型是否是 MySQL 的重复条目错误

	//使用 errors.As 函数检查错误类型是否是 MySQL 的重复条目错误，如果是则返回 true，否则返回 false
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062  //MySQL 的错误代码 1062 表示重复条目错误，检查错误代码是否匹配以确定是否是重复条目错误
	}

	return false
}
