package conf

import (
	baseconf "github.com/asynccnu/ccnubox-be/common/bizpkg/conf"
	"github.com/google/wire"
)

const ServerEnv = "CCNUBOX_CLASS_NACOS_DSN"

var ProviderSet = wire.NewSet(InitInfraConfig, InitServerConf)

type InfraConf struct {
	*baseconf.InfraConf `mapstructure:",squash"`
}

type ServerConf struct {
	baseconf.BaseServerConf `mapstructure:",squash"`
	Class                   *ClassConf `yaml:"class" mapstructure:"class"`
}

type ClassConf struct {
	HTTP                 *HTTPConf                `yaml:"http" mapstructure:"http"`
	Elasticsearch        *ElasticsearchPolicyConf `yaml:"elasticsearch" mapstructure:"elasticsearch"`
	ProxyStudentID       string                   `yaml:"proxyStudentID" mapstructure:"proxyStudentID"`
	SelectionUploadToken string                   `yaml:"selectionUploadToken" mapstructure:"selectionUploadToken"`
	DataAliveDays        int                      `yaml:"dataAliveDays" mapstructure:"dataAliveDays"`
}

type HTTPConf struct {
	Addr string `yaml:"addr" mapstructure:"addr"`
}

type ElasticsearchPolicyConf struct {
	KeepDataAfterRestart bool `yaml:"keepDataAfterRestart" mapstructure:"keepDataAfterRestart"`
}

func InitServerConf() *ServerConf {
	return baseconf.InitConfig[ServerConf](ServerEnv,
		"./config/config.yaml", "./config.yaml", "/data/conf/config.yaml")
}

func InitInfraConfig() *InfraConf {
	return &InfraConf{baseconf.InitInfraConfig(
		"./config/config-infra.yaml", "./config-infra.yaml", "/data/conf/config-infra.yaml")}
}
