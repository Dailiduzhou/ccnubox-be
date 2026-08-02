package tool

import (
	"time"

	ctool "github.com/asynccnu/ccnubox-be/common/tool"
)

func GetXnmAndXqm(currentTime time.Time) (xnm, xqm string) {
	return ctool.GetCurrentAcademicYearAndSemesterStr(currentTime)
}
