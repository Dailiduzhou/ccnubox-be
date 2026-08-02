package biz

type Error struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e *Error) Error() string { return e.Msg }

func NewError(code int, msg string) *Error { return &Error{Code: code, Msg: msg} }

var (
	ErrESAddClassInfo      = NewError(450, "创建课程信息失败")
	ErrESSearchClassInfo   = NewError(451, "查询课程信息失败")
	ErrFreeClassroomSearch = NewError(452, "查询空闲教室失败")
	ErrCCNULogin           = NewError(453, "获取教务系统登录态失败")
)
