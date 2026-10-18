package grade

type GetRankByTermReq struct {
	// 学年学期四个字段为空则获取总成绩
	XnmBegin int64 `json:"xnm_begin"`
	XqmBegin int64 `json:"xqm_begin"`
	XnmEnd   int64 `json:"xnm_end"`
	XqmEnd   int64 `json:"xqm_end"`
	Refresh  bool  `json:"refresh"`
}
