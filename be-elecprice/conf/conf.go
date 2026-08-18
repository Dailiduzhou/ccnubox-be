package conf

import (
	"fmt"
	"log"
	"strings"

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

// JnbConf JNB 能源易支付平台 V2 接口配置, 全部字段必填, 由 Nacos 或本地配置文件提供
type JnbConf struct {
	BaseUrl      string `yaml:"baseUrl"`
	SysId        string `yaml:"sysId"`
	AccountId    string `yaml:"accountId"`
	AccountPass  string `yaml:"accountPass"`
	Sm2PublicKey string `yaml:"sm2PublicKey"`
}

// Validate 校验配置完整性, 缺失必填字段时返回错误
func (c *JnbConf) Validate() error {
	if c == nil {
		return fmt.Errorf("conf: jnb 配置缺失, 请在 Nacos 或本地配置文件中提供 jnb 段")
	}
	var missing []string
	if c.BaseUrl == "" {
		missing = append(missing, "baseUrl")
	}
	if c.SysId == "" {
		missing = append(missing, "sysId")
	}
	if c.AccountId == "" {
		missing = append(missing, "accountId")
	}
	if c.AccountPass == "" {
		missing = append(missing, "accountPass")
	}
	if c.Sm2PublicKey == "" {
		missing = append(missing, "sm2PublicKey")
	}
	if len(missing) > 0 {
		return fmt.Errorf("conf: jnb 配置缺少必填字段: %s", strings.Join(missing, ", "))
	}
	return nil
}

func InitServerConf() *ServerConf {
	cfg := conf.InitConfig[ServerConf](ServerEnv)
	if err := cfg.Jnb.Validate(); err != nil {
		log.Fatalf("初始化服务配置失败: %v", err)
	}
	return cfg
}

func InitInfraConfig() *InfraConf {
	return &InfraConf{conf.InitInfraConfig()}
}
