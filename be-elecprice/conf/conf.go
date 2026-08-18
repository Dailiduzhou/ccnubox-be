package conf

import (
	"github.com/asynccnu/ccnubox-be/common/bizpkg/conf"
)

const (
	ServerEnv = "CCNUBOX_ELECPRICE_NACOS_DSN"
)

// InfraConf 通用配置
type InfraConf struct {
	*conf.InfraConf `mapstructure:",squash"` //为了能够正常解析需要对其进行拍平
}

// ServerConf 服务配置
type ServerConf struct {
	conf.BaseServerConf `mapstructure:",squash"`
	ElecpriceController *ElecPriceConf `yaml:"elecpriceController"`
	Jnb                 *JnbConf       `yaml:"jnb"`
}

type ElecPriceConf struct {
	DurationTime int `yaml:"durationTime"`
}

// JnbConf JNB 能源易支付平台 V2 接口配置
type JnbConf struct {
	BaseUrl      string `yaml:"baseUrl"`
	SysId        string `yaml:"sysId"`
	AccountId    string `yaml:"accountId"`
	AccountPass  string `yaml:"accountPass"`
	Sm2PublicKey string `yaml:"sm2PublicKey"`
}

// Default 返回带默认值的配置, 未配置的项使用生产默认值
func (c *JnbConf) Default() *JnbConf {
	res := &JnbConf{
		BaseUrl:      "https://jnb.ccnu.edu.cn/ICBS_V2_Server",
		SysId:        "999",
		AccountId:    "ph",
		AccountPass:  "phAPI",
		Sm2PublicKey: "04b238b7d42c87a25e1a4eaddca81e8f33fd95773ccd471408e4195db62aa4085f6e0a3b695cad8d60acece1af348f534b7f72d312f2966b144248c9c590c930fc",
	}
	if c == nil {
		return res
	}
	if c.BaseUrl != "" {
		res.BaseUrl = c.BaseUrl
	}
	if c.SysId != "" {
		res.SysId = c.SysId
	}
	if c.AccountId != "" {
		res.AccountId = c.AccountId
	}
	if c.AccountPass != "" {
		res.AccountPass = c.AccountPass
	}
	if c.Sm2PublicKey != "" {
		res.Sm2PublicKey = c.Sm2PublicKey
	}
	return res
}

func InitServerConf() *ServerConf {
	return conf.InitConfig[ServerConf](ServerEnv)
}

func InitInfraConfig() *InfraConf {
	return &InfraConf{conf.InitInfraConfig()}
}
