package ioc

import (
	"github.com/asynccnu/ccnubox-be/be-class/conf"
	commonlog "github.com/asynccnu/ccnubox-be/common/bizpkg/log"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
)

func InitLogger(cfg *conf.ServerConf) logger.Logger { return commonlog.InitLogger(cfg.Log, 3) }
