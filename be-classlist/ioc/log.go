package ioc

import (
	"strings"

	"github.com/asynccnu/ccnubox-be/be-classlist/conf"
	"github.com/asynccnu/ccnubox-be/be-classlist/pkg/tool"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/log"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
)

func InitLogger(cfg *conf.ServerConf) logger.Logger {
	res := log.InitLogger(cfg.Log, 3)

	// 结构化字段兜底脱敏：只要 key 是学号相关，无论是否已手动脱敏都再走一遍 MaskStudentID
	// （MaskStudentID 幂等，已脱敏的值不会二次变化）
	return logger.NewFilterLogger(res,
		logger.FilterFunc(func(level logger.Level, key, val string) (string, bool) {
			k := strings.ToLower(key)
			if strings.Contains(k, "stu_id") || strings.Contains(k, "student_id") {
				return tool.MaskStudentID(val), true
			}
			return val, false
		}),
	)
}
