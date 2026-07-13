package database

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/model"
)

var DB *gorm.DB

func InitMySQL(cfg *config.Config) error {
	mysqlCfg := cfg.MySQL

	//DSN连接MySQL数据库
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		mysqlCfg.Username,
		mysqlCfg.Password,
		mysqlCfg.Host,
		mysqlCfg.Port,
		mysqlCfg.Database,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	DB = db

	return nil
}

// AutoMigrate 自动创建或更新表结构
func AutoMigrate() error {
	if DB == nil {
		return fmt.Errorf("mysql db is not initialized")
	}

	if err := DB.AutoMigrate(
		&model.Project{},
		&model.ProjectFile{},
		&model.CodeChunk{},
		&model.DiffRecord{},
		&model.RiskReport{},
		&model.AnalysisTask{},
	); err != nil {
		return err
	}

	return nil
}
