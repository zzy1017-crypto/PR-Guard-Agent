package database

import (
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB //全局数据库连接对象，程序中其他地方可以通过database.DB访问数据库连接

// InitMySQL 连接MySQL数据库，使用gorm连接MySQL，并将连接对象保存在DB变量中
func InitMySQL() error {

	dsn := os.Getenv("MYSQL_DSN")

	//使用gorm连接MySQL数据库，dsn是数据源名称，包含了连接信息，如用户名、密码、地址、数据库名等
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	DB = db //将连接对象保存在全局变量DB中，程序中其他地方可以通过database.DB访问数据库连接

	return nil
}
