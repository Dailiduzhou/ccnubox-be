package grade

import (
	"errors"

	"github.com/asynccnu/ccnubox-be/bff/errs"
	"github.com/asynccnu/ccnubox-be/bff/web"
	"github.com/asynccnu/ccnubox-be/bff/web/ijwt"
	"github.com/gin-gonic/gin"
)

// GetRankByTerm 查询学分绩排名（已下线）
// @Summary 查询学分绩排名（已下线）
// @Description 兼容旧客户端，固定返回功能已下线，不再查询校方排名
// @Tags grade
// @Produce json
// @Failure 410 {object} web.Response "排名功能已下线，code=41001"
// @Deprecated
// @Router /grade/getRankByTerm [get]
func (h *GradeHandler) GetRankByTerm(ctx *gin.Context, req GetRankByTermReq, uc ijwt.UserClaims) (web.Response, error) {
	return web.Response{}, errs.RANK_OFFLINE_ERROR(errors.New("排名功能已下线"))
}

// LoadRank 预加载学分绩排名（已下线）
// @Summary 预加载总排名（已下线）
// @Description 兼容旧客户端，固定返回功能已下线，不再启动预加载
// @Tags grade
// @Produce json
// @Failure 410 {object} web.Response "排名功能已下线，code=41001"
// @Deprecated
// @Router /grade/loadRank [get]
func (h *GradeHandler) LoadRank(ctx *gin.Context, uc ijwt.UserClaims) (web.Response, error) {
	return web.Response{}, errs.RANK_OFFLINE_ERROR(errors.New("排名功能已下线"))
}
