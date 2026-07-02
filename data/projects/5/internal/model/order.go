package model

import "time"

type Order struct {
	ID uint `gorm:"primaryKey"`  //订单ID，主键，自增

	OrderID string `gorm:"type:varchar(64);uniqueIndex;not null"` //订单编号，唯一索引，不能为空

	UserID string `gorm:"type:varchar(64);not null;uniqueIndex:idx_user_product"` //添加唯一索引，确保同一用户对同一商品只能有一个订单

	ProductID string `gorm:"type:varchar(64);not null;uniqueIndex:idx_user_product"`//添加唯一索引，确保同一用户对同一商品只能有一个订单

	Status string `gorm:"type:varchar(32);not null;default:created"` //订单状态，默认为created，表示订单已创建，后续可以更新为paid、shipped、completed等状态

	CreatedAt time.Time //订单创建时间，gorm会自动管理这个字段，记录订单的创建时间
}
