package database

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/model"
)

var DB *gorm.DB // DB是一个全局变量，用于存储数据库连接实例，类型为*gorm.DB。

// InitMySQL 初始化MySQL数据库连接
func InitMySQL(cfg *config.Config) error {
	mysqlCfg := cfg.MySQL // 获取配置中的MySQL配置参数，类型为config.MySQLConfig。

	//DSN连接MySQL数据库
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		mysqlCfg.Username,
		mysqlCfg.Password,
		mysqlCfg.Host,
		mysqlCfg.Port,
		mysqlCfg.Database,
	)

	// 使用GORM的mysql驱动打开数据库连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open mysql connection failed: %w", err)
	}

	DB = db // 将数据库连接实例赋值给全局变量DB，以便在其他地方使用。

	return nil
}

// AutoMigrate 自动创建或更新表结构
func AutoMigrate() error {

	// 检查全局变量DB是否为nil，如果是nil，说明数据库连接未初始化，返回错误。
	if DB == nil {
		return fmt.Errorf("mysql db is not initialized")
	}

	// 调用GORM的AutoMigrate方法，自动创建或更新数据库表结构。传入的参数是需要迁移的模型结构体。
	if err := DB.AutoMigrate(
		&model.Project{},
		&model.ProjectFile{},
		&model.CodeChunk{},
		&model.DiffRecord{},
		&model.RiskReport{},
		&model.AnalysisTask{},
	); err != nil {
		return fmt.Errorf("auto migrate mysql schema failed: %w", err)
	}

	return nil
}
