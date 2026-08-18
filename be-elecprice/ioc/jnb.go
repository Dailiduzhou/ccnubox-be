package ioc

import (
	"github.com/asynccnu/ccnubox-be/be-elecprice/conf"
	"github.com/asynccnu/ccnubox-be/be-elecprice/crawler"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/proxy"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
)

func InitJnbClient(pc proxy.Client, cfg *conf.ServerConf, l logger.Logger) crawler.JnbClient {
	return crawler.NewJnbClient(pc, l, cfg.Jnb)
}
