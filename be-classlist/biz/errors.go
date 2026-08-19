package biz

import "errors"

var (
	ErrClassNotFound         = errors.New("class not found")
	ErrClassAlreadyExists    = errors.New("class already exists")
	ErrClassScheduleConflict = errors.New("class schedule conflict")
	ErrStudentCourseNotFound = errors.New("student course relation not found")
	ErrInvalidParam          = errors.New("invalid param")
	ErrClassDeleteRejected   = errors.New("class delete rejected")
	ErrClassUpdateRejected   = errors.New("class update rejected")
	ErrGetStuIDsByJxbID      = errors.New("get student ids by jxb id failed")

	// ErrClassRefreshPending 表示另一个刷新任务在请求等待预算内仍未完成。
	// 它描述“暂时没有可返回结果”，只用于同步查询返回，不会据此创建新的异步重试消息。
	ErrClassRefreshPending = errors.New("class refresh is still pending")

	// ErrRefreshPersistence 表示刷新流水线写入刷新日志、课程快照或清理冲突课程失败。
	ErrRefreshPersistence = errors.New("class refresh persistence failed")

	// ErrCrawlerTemporary 表示上游明确返回的临时故障，例如 408/425/429 或 5xx。
	ErrCrawlerTemporary = errors.New("temporary class crawler failure")

	// ErrCrawlerAuthentication 表示 Cookie 为空、被重定向到登录页或上游返回 401/403。
	// 使用相同登录态重试不会恢复，所以该错误是永久错误，不应自动重试。
	ErrCrawlerAuthentication = errors.New("class crawler authentication failed")

	// ErrCrawlerProtocol 表示响应格式、字段、状态码或课表时间描述不符合当前协议。
	// 这通常意味着上游接口变化或程序解析规则过期，重复请求不会修复，应停止重试并保留根因。
	ErrCrawlerProtocol = errors.New("class crawler protocol failed")

	// ErrCrawlerEmptyResult 表示上游调用成功，但没有得到当前业务所要求的课程和选课关系。
	// 它作为不可重试的业务/协议异常，避免持续请求学校系统。
	ErrCrawlerEmptyResult = errors.New("class crawler returned empty result")

	// ErrCookieUnavailable 表示用户服务成功响应却没有可用响应对象或 Cookie。
	// 这不是传输瞬时错误；若用户服务返回了 gRPC 错误，则保留原错误并由 gRPC code 决定是否重试。
	ErrCookieUnavailable = errors.New("user cookie is unavailable")

	// ErrUnsupportedStudentType 表示学号无法映射到当前支持的本科生或研究生爬虫。
	// 输入不变时重试结果也不会改变，因此它是不可重试错误。
	ErrUnsupportedStudentType = errors.New("unsupported student type")

	// ErrRefreshInvariant 表示刷新代码违反内部不变量，例如 singleflight 返回了错误类型。
	ErrRefreshInvariant = errors.New("class refresh invariant violated")
)
