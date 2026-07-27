package biz

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asynccnu/ccnubox-be/be-class/internal/lock"
	clog "github.com/asynccnu/ccnubox-be/be-class/internal/log"
	"github.com/asynccnu/ccnubox-be/be-class/internal/model"
	v1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/classlist/v1"
)

type EsProxy interface {
	AddClassInfo(ctx context.Context, classInfo ...model.ClassInfo) error
	ClearClassInfo(ctx context.Context, xnm, xqm string)
	SearchClassInfo(ctx context.Context, keyWords string, xnm, xqm string, page, pageSize int) ([]model.ClassInfo, error)
}

type ClassListService interface {
	GetAllSchoolClassInfos(ctx context.Context, xnm, xqm, cursor string) ([]model.ClassInfo, string, error)
	AddClassInfoToClassListService(ctx context.Context, req *v1.AddClassRequest) (*v1.AddClassResponse, error)
}

type ClassServiceUserCase struct {
	es          EsProxy
	cs          ClassListService
	lockBuilder lock.Builder
	cache       Cache
}

func NewClassServiceUserCase(es EsProxy, cs ClassListService, lockBuilder lock.Builder, cache Cache) *ClassServiceUserCase {
	return &ClassServiceUserCase{
		es:          es,
		cs:          cs,
		lockBuilder: lockBuilder,
		cache:       cache,
	}
}

func (c *ClassServiceUserCase) AddClassInfoToClassListService(ctx context.Context, request *v1.AddClassRequest) (*v1.AddClassResponse, error) {
	return c.cs.AddClassInfoToClassListService(ctx, request)
}

func (c *ClassServiceUserCase) SearchClassInfo(ctx context.Context, keyWords string, xnm, xqm string, page, pageSize int) ([]model.ClassInfo, error) {
	return c.es.SearchClassInfo(ctx, keyWords, xnm, xqm, page, pageSize)
}

func (c *ClassServiceUserCase) AddClassInfosToES(ctx context.Context, xnm, xqm string) error {
	//xnm, xqm := tool.GetXnmAndXqm()
	reqTime := "1949-10-01T00:00:00.000000"
	var tasks []string
	var syncedAny bool

	defer func() {
		_ = c.cache.Del(ctx, tasks...)
	}()

	for {
		classInfos, lastTime, err := c.cs.GetAllSchoolClassInfos(ctx, xnm, xqm, reqTime)
		if err != nil {
			return fmt.Errorf("failed to get all classlist (year=%s semester=%s cursor=%s): %w", xnm, xqm, reqTime, err)
		}
		if len(classInfos) == 0 {
			if !syncedAny {
				return fmt.Errorf("classlist service returned no classes for year=%s semester=%s", xnm, xqm)
			}
			return nil
		}

		// 使用分布式锁来确保只有一个实例在执行
		lockKey := fmt.Sprintf("add_classlist_to_es_%v_%v_%v", xnm, xqm, reqTime)
		locker := c.lockBuilder.Build(lockKey)

		err = locker.Lock()

		if err != nil {
			return fmt.Errorf("failed to acquire class sync lock %s: %w", lockKey, err)
		}

		// 成功获取到锁
		clog.LogPrinter.Infof("get the lock: %v", lockKey)

		// 应该标识下任务是否完成
		// 如果任务已经完成了,应该接着看下一个
		taskName := "task:" + lockKey
		tasks = append(tasks, taskName)

		status, err := c.cache.Get(ctx, taskName)
		if err == nil && status == Finished {
			syncedAny = true
			if err := unlockClassSync(locker, lockKey); err != nil {
				return err
			}

			reqTime = lastTime
			continue
		}

		err = c.es.AddClassInfo(ctx, classInfos...)
		if err != nil {
			err1 := c.cache.Set(ctx, taskName, Failed, 10*time.Minute)
			if err1 != nil {
				clog.LogPrinter.Errorf("failed to set %v %v", taskName, err1)
			}
			clog.LogPrinter.Errorf("add classlist[%v] failed: %v", classInfos, err)
			syncErr := fmt.Errorf("failed to add %d classes to es: %w", len(classInfos), err)
			if unlockErr := unlockClassSync(locker, lockKey); unlockErr != nil {
				return errors.Join(syncErr, unlockErr)
			}
			return syncErr
		}
		syncedAny = true

		clog.LogPrinter.Infof("es has save %d classes", len(classInfos))

		err = c.cache.Set(ctx, taskName, Finished, 10*time.Minute)
		if err != nil {
			clog.LogPrinter.Errorf("failed to set %v %v", taskName, err)
		}

		if err := unlockClassSync(locker, lockKey); err != nil {
			return err
		}

		reqTime = lastTime
	}
}

func unlockClassSync(locker lock.Locker, lockKey string) error {
	ok, err := locker.Unlock()
	if err != nil {
		return fmt.Errorf("failed to unlock class sync lock %s: %w", lockKey, err)
	}
	if !ok {
		return fmt.Errorf("failed to unlock class sync lock %s: lock was not released", lockKey)
	}
	clog.LogPrinter.Infof("unlock %v successfully", lockKey)
	return nil
}

func (c *ClassServiceUserCase) DeleteSchoolClassInfosFromES(ctx context.Context, xnm, xqm string) {
	//xnm, xqm := tool.GetXnmAndXqm()
	c.es.ClearClassInfo(ctx, xnm, xqm)
}
