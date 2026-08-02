package tool

import (
	"strconv"
	"time"
)

// GetCurrentAcademicYearAndSemester 根据时间粗略获取当前学年和学期
func GetCurrentAcademicYearAndSemester(now time.Time) (int, int) {
	year := now.Year()
	month := int(now.Month())

	switch {
	case month >= 9: // 9-12月
		return year, 1

	case month == 1: // 1月
		return year - 1, 1

	case month >= 2 && month <= 6: // 2-6月
		return year - 1, 2

	case month >= 7 && month <= 8: // 7-8月
		return year - 1, 3
	}

	return year, 1 // 理论不会走到这里
}

func GetCurrentAcademicYearAndSemesterStr(now time.Time) (string, string) {
	y, s := GetCurrentAcademicYearAndSemester(now)
	return strconv.Itoa(y), strconv.Itoa(s)
}
