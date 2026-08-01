package user

import (
	"context"
	"strconv"
	"time"

	classlistv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/classlist/v1"
	feedv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/feed/v1"
	gradev1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/grade/v1"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/spf13/viper"
)

type PreLoader interface {
	PreLoad(ctx context.Context, studentId string)
}

func NewPreLoader(
	gradeClient gradev1.GradeServiceClient,
	classerClient classlistv1.ClasserClient,
	feedClient feedv1.FeedServiceClient,
	l logger.Logger,
) PreLoader {

	return &preLoader{
		gradeClient:     gradeClient,
		classerClient:   classerClient,
		feedClient:      feedClient,
		l:               l,
		currentSemester: viper.GetString("classlist.currentSemester"),
	}
}

type preLoader struct {
	gradeClient     gradev1.GradeServiceClient
	classerClient   classlistv1.ClasserClient
	feedClient      feedv1.FeedServiceClient
	l               logger.Logger
	currentSemester string
}

const preloadTimeout = 15 * time.Second

func (l *preLoader) PreLoad(ctx context.Context, studentId string) {
	// 预创建feed的配置列表
	runPreload(ctx, func(ctx context.Context) {
		_, _ = l.feedClient.FindOrCreateAllowList(ctx, &feedv1.FindOrCreateAllowListReq{StudentId: studentId})
	})

	// 异步获取学生成绩,不需要等待结果
	runPreload(ctx, func(ctx context.Context) {
		_, _ = l.gradeClient.GetGradeScore(ctx, &gradev1.GetGradeScoreReq{
			StudentId: studentId,
		})
	})

	runPreload(ctx, func(ctx context.Context) {
		_, _ = l.gradeClient.GetGradeByTerm(ctx, &gradev1.GetGradeByTermReq{
			StudentId: studentId,
			Refresh:   true,
			Kcxzmcs:   []string{"1"},
		})
	})

	// 异步获取学生课表,不需要等待结果
	runPreload(ctx, func(ctx context.Context) {
		_, _ = l.classerClient.GetClass(ctx, &classlistv1.GetClassRequest{
			Refresh:  true,
			StuId:    studentId,
			Year:     academicYear(time.Now()),
			Semester: l.currentSemester,
		})
	})
}

func runPreload(parent context.Context, task func(context.Context)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), preloadTimeout)
		defer cancel()
		task(ctx)
	}()
}

func academicYear(now time.Time) string {
	year := now.Year()
	if now.Month() < time.September {
		year--
	}
	return strconv.Itoa(year)
}
