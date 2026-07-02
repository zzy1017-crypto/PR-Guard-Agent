package logger

import (
	"go.uber.org/zap" //全局日志对象
)

var Log *zap.Logger

func InitLogger() {
	var err error

	Log, err = zap.NewProduction() //初始化日志对象,创建生产级logger

	if err != nil {
		panic(err) //如果初始化日志对象失败，直接panic
	}
}
