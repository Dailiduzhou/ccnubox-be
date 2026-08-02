package cron

import (
	"context"
	"strconv"
	"time"

	"github.com/asynccnu/ccnubox-be/be-class_v2/biz/usecase"
	"github.com/asynccnu/ccnubox-be/be-class_v2/client"
	"github.com/asynccnu/ccnubox-be/be-class_v2/conf"
	"github.com/asynccnu/ccnubox-be/be-class_v2/pkg/semesterinfo"
	"github.com/asynccnu/ccnubox-be/be-class_v2/pkg/tool"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/google/wire"
	"github.com/robfig/cron/v3"
)

var ProviderSet = wire.NewSet(NewTask)

// Task 定义 Task 结构体
type Task struct {
	classServiceUserCase *usecase.ClassServiceUserCase
	freeClassroomBiz     *usecase.FreeClassroomBiz
	classlistService     *client.ClassListService
	config               *conf.ServerConf
	logger               logger.Logger
	c                    *cron.Cron
}

func NewTask(classServiceUserCase *usecase.ClassServiceUserCase, freeClassroomBiz *usecase.FreeClassroomBiz, classlistService *client.ClassListService, cfg *conf.ServerConf, l logger.Logger) *Task {
	return &Task{
		classServiceUserCase: classServiceUserCase,
		freeClassroomBiz:     freeClassroomBiz,
		classlistService:     classlistService,
		config:               cfg,
		logger:               l,
		c:                    cron.New(),
	}
}

// RegisterAddClassInfosToESTask 实现 Task 的 RegisterAddClassInfosToESTask 方法
func (t Task) RegisterAddClassInfosToESTask() {
	ctx := context.Background()
	//程序开始时先执行一次
	go func() {
		xnm, xqm := tool.GetXnmAndXqm(time.Now())
		for attempt := 1; attempt <= 3; attempt++ {
			if err := t.syncLocalClassroomData(ctx, xnm, xqm); err == nil {
				return
			} else {
				t.logger.Errorf("sync local classroom data failed (year=%s semester=%s attempt=%d): %v", xnm, xqm, attempt, err)
			}
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 10 * time.Second)
			}
		}
	}()

	// 每天凌晨 3 点执行
	err := t.AddTask("0 3 * * *", func() {
		xnm, xqm := tool.GetXnmAndXqm(time.Now())
		if err := t.syncLocalClassroomData(ctx, xnm, xqm); err != nil {
			t.logger.Errorf("scheduled local classroom sync failed (year=%s semester=%s): %v", xnm, xqm, err)
		}
	})
	if err != nil {
		panic(err)
	}
}

func (t Task) syncLocalClassroomData(ctx context.Context, xnm, xqm string) error {
	t.logger.Infof("start syncing class data to es (year=%s semester=%s)", xnm, xqm)
	if err := t.classServiceUserCase.AddClassInfosToES(ctx, xnm, xqm); err != nil {
		return err
	}

	// Elasticsearch is near-real-time. Wait for its refresh before deriving occupancy documents.
	time.Sleep(5 * time.Second)
	t.logger.Infof("start deriving classroom occupancy (year=%s semester=%s)", xnm, xqm)
	return t.freeClassroomBiz.SaveFreeClassRoomFromLocal(ctx, xnm, xqm)
}

// RegisterClearClassInfoTask 清洁任务
func (t Task) RegisterClearClassInfoTask() {
	ctx := context.Background()

	// 每天凌晨 5 点清理非当前学期数据。
	err := t.AddTask("0 5 * * *", func() {
		t.logger.Info("开始执行 ClearClassInfo 任务")
		xnm, xqm := tool.GetXnmAndXqm(time.Now())
		t.classServiceUserCase.DeleteSchoolClassInfosFromES(ctx, xnm, xqm)
		if err := t.freeClassroomBiz.ClearClassroomOccupancyFromES(ctx, xnm, xqm); err != nil {
			t.logger.Errorf("ClearClassroomOccupancyFromES failed (year=%s semester=%s): %v", xnm, xqm, err)
		}
	})
	if err != nil {
		panic(err)
	}
}

func (t Task) RegisterCrawFreeClassroomTask(stuId string) {
	ctx := context.Background()
	go func() {
		schoolTime, err := t.classlistService.GetSchoolDay(ctx)
		if err != nil {
			t.logger.Errorf("get school day failed: %v", err)
			return
		}
		si, err := semesterInfo.GetSemesterInfo(schoolTime)
		if err != nil {
			t.logger.Errorf("get semester info failed: %v", err)
			return
		}
		// 程序开始时先执行一次
		t.freeClassroomBiz.LoadOneWeekFreeClassRoom(ctx, stuId, strconv.Itoa(si.Year), strconv.Itoa(si.Semester), si.WeekNumber)
	}()

	// 每周一4点执行
	err := t.AddTask("0 4 * * 1", func() {
		schoolTime, err := t.classlistService.GetSchoolDay(ctx)
		if err != nil {
			t.logger.Errorf("get school day failed: %v", err)
			return
		}
		si, err := semesterInfo.GetSemesterInfo(schoolTime)
		if err != nil {
			t.logger.Errorf("get semester info failed: %v", err)
			return
		}
		t.freeClassroomBiz.LoadOneWeekFreeClassRoom(ctx, stuId, strconv.Itoa(si.Year), strconv.Itoa(si.Semester), si.WeekNumber)
	})
	if err != nil {
		panic(err)
	}
}

// AddTask 用于添加定时任务
func (t Task) AddTask(spec string, task func()) error {
	_, err := t.c.AddFunc(spec, task)
	if err != nil {
		t.logger.Errorf("failed to add  short task")
		return err
	}
	return nil
}

func (t Task) Start() {
	t.c.Start()
}

func (t Task) StartAll() {
	t.RegisterAddClassInfosToESTask()
	t.RegisterClearClassInfoTask()
	t.RegisterCrawFreeClassroomTask(t.config.Class.ProxyStudentID)
	t.Start()
}

func (t Task) Stop() {
	ctx := t.c.Stop()
	select {
	case <-ctx.Done():
		t.logger.Info("所有定时任务已停止")
	}
}
