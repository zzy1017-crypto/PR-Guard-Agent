package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"` //服务器配置
	MySQL  MySQLConfig  `mapstructure:"mysql"`  //数据库配置
	Redis  RedisConfig  `mapstructure:"redis"`  //Redis配置
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

var GlobalConfig Config    //全局配置变量，程序中其他地方可以通过config.GlobalConfig访问配置项

// InitConfig 从config.yaml加载配置到GlobalConfig中
func InitConfig() error {

	viper.SetConfigName("config") //配置文件名为config
	viper.SetConfigType("yaml")  //配置文件类型为yaml
	viper.AddConfigPath(".")     //配置文件的搜索路径，在这指当前目录

	viper.AutomaticEnv()         //自动从环境变量加载配置，支持覆盖config.yaml中的配置项

	//读取配置文件，如果读取失败，返回错误
	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	//将读取到的配置文件内容反序列化到GlobalConfig结构体中，如果反序列化失败，返回错误
	err := viper.Unmarshal(&GlobalConfig)
	if err != nil {
		return err
	}

	return nil
}
