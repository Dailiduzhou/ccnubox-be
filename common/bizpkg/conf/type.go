package conf

type Env string

const (
	EnvDev  Env = "dev"
	EnvTest Env = "test"
	EnvProd Env = "prod"
	EnvGrey Env = "grey"
)

// String 转换方法
func (e Env) String() string {
	return string(e)
}

// IsProd 判断是否为生产环境
func (e Env) IsProd() bool {
	return e == EnvProd
}

func (e Env) IsDev() bool {
	return e == EnvDev
}

// InfraConf 基础配置
type InfraConf struct {
	Env           *Env               `yaml:"env"`
	Etcd          *EtcdConf          `yaml:"etcd"`
	Redis         *RedisConf         `yaml:"redis"`
	Mysql         *MysqlConf         `yaml:"mysql"`
	Kafka         *KafkaConf         `yaml:"kafka"`
	Grpc          *GrpcConfs         `yaml:"grpc"`
	Otel          *OtelConf          `yaml:"otel"`
	Proxy         *ProxyConf         `yaml:"proxy"`
	Elasticsearch *ElasticsearchConf `yaml:"elasticsearch"`
}

type EtcdConf struct {
	Endpoints []string `yaml:"endpoints"`
	Username  string   `yaml:"username"`
	Password  string   `yaml:"password"`
}

type RedisConf struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
}

type MysqlConf struct {
	DSN                    string `yaml:"dsn"`
	MaxOpenConns           int    `yaml:"maxOpenConns"`
	MaxIdleConns           int    `yaml:"maxIdleConns"`
	ConnMaxLifetimeSeconds int    `yaml:"connMaxLifetimeSeconds"`
	ConnMaxIdleTimeSeconds int    `yaml:"connMaxIdleTimeSeconds"`
}

type KafkaConf struct {
	Addrs    []string `yaml:"addrs"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
}

type ElasticsearchConf struct {
	URLs     []string `yaml:"urls"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	Sniff    bool     `yaml:"sniff"`
}

type ProxyConf struct {
	Mode string `yaml:"mode"`
}

func (p *ProxyConf) IsDirect() bool {
	return p != nil && p.Mode == "direct"
}

type LogConf struct {
	Path       string `yaml:"path"`
	MaxSize    int    `yaml:"maxSize"`
	MaxBackups int    `yaml:"maxBackups"`
	MaxAge     int    `yaml:"maxAge"`
	Compress   bool   `yaml:"compress"`
}

// BaseServerConf
type BaseServerConf struct {
	Log *LogConf `yaml:"log"`
}

// GRPC

// GrpcConfs 保留共享服务目录的 YAML 结构，运行时分别转换为客户端和服务端 options。
type GrpcConfs map[string]*GrpcConf

type GrpcConf struct {
	Name          string `yaml:"name"`
	Weight        int    `yaml:"weight"`
	Addr          string `yaml:"addr"`
	EtcdTTL       int    `yaml:"etcdTTL"`
	ServerTimeout int    `yaml:"serverTimeout"`
	ClientTimeout int    `yaml:"clientTimeout"`
}

// OTel
type OtelConf struct {
	ServiceName string `yaml:"serviceName"`
	Endpoint    string `yaml:"endpoint"`
}
