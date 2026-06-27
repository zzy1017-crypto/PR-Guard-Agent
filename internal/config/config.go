package config

import "github.com/spf13/viper"

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	MySQL  MySQLConfig  `mapstructure:"mysql"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Qdrant QdrantConfig `mapstructure:"qdrant"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type QdrantConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	GRPCPort int    `mapstructure:"grpc_port"`
}

func Load(path string) (*Config, error) {
	v := viper.New()                             //创建一个新的Viper实例，用于加载和解析配置文件
	v.SetConfigFile(path)                        //设置配置文件的路径，path是传入的参数，表示配置文件的具体位置
	v.SetConfigType("yaml")                      //设置配置文件的类型为YAML，这样Viper就知道如何解析配置文件的内容
	v.SetDefault("server.port", 8080)            //设置默认的服务器端口为8080，如果配置文件中没有指定端口号，就会使用这个默认值
	v.SetDefault("server.mode", "debug")         //设置默认的服务器模式为"debug"，如果配置文件中没有指定模式，就会使用这个默认值
	v.SetDefault("mysql.host", "localhost")      //设置默认的MySQL数据库主机为"localhost"，如果配置文件中没有指定主机，就会使用这个默认值
	v.SetDefault("mysql.port", 3306)             //设置默认的MySQL数据库端口为3306，如果配置文件中没有指定端口，就会使用这个默认值
	v.SetDefault("mysql.database", "pr_guard")   //设置默认的MySQL数据库名称为"pr_guard"，如果配置文件中没有指定数据库名称，就会使用这个默认值
	v.SetDefault("mysql.username", "root")       //设置默认的MySQL数据库用户名为"root"，如果配置文件中没有指定用户名，就会使用这个默认值
	v.SetDefault("mysql.password", "123456")     //设置默认的MySQL数据库密码为"123456"，如果配置文件中没有指定密码，就会使用这个默认值
	v.SetDefault("redis.addr", "localhost:6379") //设置默认的Redis数据库地址为"localhost:6379"，如果配置文件中没有指定地址，就会使用这个默认值
	v.SetDefault("redis.password", "")           //设置默认的Redis数据库密码为空字符串，如果配置文件中没有指定密码，就会使用这个默认值
	v.SetDefault("redis.db", 0)                  //设置默认的Redis数据库索引为0，如果配置文件中没有指定索引，就会使用这个默认值
	v.SetDefault("qdrant.host", "localhost")     //设置默认的Qdrant数据库主机为"localhost"，如果配置文件中没有指定主机，就会使用这个默认值
	v.SetDefault("qdrant.port", 6333)            //设置默认的Qdrant数据库端口为6333，如果配置文件中没有指定端口，就会使用这个默认值
	v.SetDefault("qdrant.grpc_port", 6334)       //设置默认的Qdrant数据库gRPC端口为6334，如果配置文件中没有指定gRPC端口，就会使用这个默认值

	//读取配置文件，如果读取失败，返回错误信息
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	//将读取到的配置文件内容反序列化到Config结构体中，如果反序列化失败，返回错误信息
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil //返回指向Config结构体的指针，表示成功加载并解析了配置文件
}
