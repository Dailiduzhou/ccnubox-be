package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asynccnu/ccnubox-be/be-class/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class/repository/lock"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
)

type EsProxy interface {
	AddClassInfo(ctx context.Context, classInfo ...model.ClassInfo) error
	ClearClassInfo(ctx context.Context, xnm, xqm string)
	SearchClassInfo(ctx context.Context, keyWords string, xnm, xqm string, page, pageSize int) ([]model.ClassInfo, error)
}

type ClassListService interface {
	GetAllSchoolClassInfos(ctx context.Context, xnm, xqm, cursor string) ([]model.ClassInfo, string, error)
	AddClass(ctx context.Context, req model.AddClassRequest) (model.AddClassResult, error)
}

type ClassServiceUserCase struct {
	es          EsProxy
	cs          ClassListService
	lockBuilder lock.Builder
	cache       Cache
	logger      logger.Logger
}

func NewClassServiceUserCase(es EsProxy, cs ClassListService, lockBuilder lock.Builder, cache Cache, l logger.Logger) *ClassServiceUserCase {
	return &ClassServiceUserCase{
		es:          es,
		cs:          cs,
		lockBuilder: lockBuilder,
		cache:       cache,
		logger:      l,
	}
}

func (c *ClassServiceUserCase) AddClassInfoToClassListService(ctx context.Context, request model.AddClassRequest) (model.AddClassResult, error) {
	return c.cs.AddClass(ctx, request)
}

func (c *ClassServiceUserCase) SearchClassInfo(ctx context.Context, keyWords string, xnm, xqm string, page, pageSize int) ([]model.ClassInfo, error) {
	return c.es.SearchClassInfo(ctx, keyWords, xnm, xqm, page, pageSize)
}

func (c *ClassServiceUserCase) AddClassInfosToES(ctx context.Context, xnm, xqm string) error {
	//xnm, xqm := tool.GetXnmAndXqm()
	pages, err := c.loadClassInfoPages(ctx, xnm, xqm)
	if err != nil {
		return err
	}
	var tasks []string

	defer func() {
		_ = c.cache.Del(ctx, tasks...)
	}()

	for _, page := range pages {
		reqTime := page.cursor
		classInfos := page.classInfos

		// 使用分布式锁来确保只有一个实例在执行
		lockKey := fmt.Sprintf("add_classlist_to_es_%v_%v_%v", xnm, xqm, reqTime)
		locker := c.lockBuilder.Build(lockKey)

		err = locker.Lock()

		if err != nil {
			return fmt.Errorf("failed to acquire class sync lock %s: %w", lockKey, err)
		}

		// 成功获取到锁
		c.logger.Infof("get the lock: %v", lockKey)

		// 应该标识下任务是否完成
		// 如果任务已经完成了,应该接着看下一个
		taskName := "task:" + lockKey
		tasks = append(tasks, taskName)

		status, err := c.cache.Get(ctx, taskName)
		if err == nil && status == Finished {
			if err := unlockClassSync(locker, lockKey); err != nil {
				return err
			}
			continue
		}

		err = c.es.AddClassInfo(ctx, classInfos...)
		if err != nil {
			err1 := c.cache.Set(ctx, taskName, Failed, 10*time.Minute)
			if err1 != nil {
				c.logger.Errorf("failed to set %v %v", taskName, err1)
			}
			c.logger.Errorf("add classlist[%v] failed: %v", classInfos, err)
			syncErr := fmt.Errorf("failed to add %d classes to es: %w", len(classInfos), err)
			if unlockErr := unlockClassSync(locker, lockKey); unlockErr != nil {
				return errors.Join(syncErr, unlockErr)
			}
			return syncErr
		}
		c.logger.Infof("es has save %d classes", len(classInfos))

		err = c.cache.Set(ctx, taskName, Finished, 10*time.Minute)
		if err != nil {
			c.logger.Errorf("failed to set %v %v", taskName, err)
		}

		if err := unlockClassSync(locker, lockKey); err != nil {
			return err
		}
	}

	return nil
}

type classInfoPage struct {
	cursor     string
	classInfos []model.ClassInfo
}

func (c *ClassServiceUserCase) loadClassInfoPages(ctx context.Context, xnm, xqm string) ([]classInfoPage, error) {
	reqTime := "1949-10-01T00:00:00.000000"
	seenCursors := map[string]struct{}{reqTime: {}}
	var pages []classInfoPage

	// Validate the complete cursor chain before publishing any page. A failure on
	// a later page must not leave Elasticsearch with an incomplete sync result.
	for {
		classInfos, lastTime, err := c.cs.GetAllSchoolClassInfos(ctx, xnm, xqm, reqTime)
		if err != nil {
			return nil, fmt.Errorf("failed to get all classlist (year=%s semester=%s cursor=%s): %w", xnm, xqm, reqTime, err)
		}
		if len(classInfos) == 0 {
			if len(pages) == 0 {
				return nil, fmt.Errorf("classlist service returned no classes for year=%s semester=%s", xnm, xqm)
			}
			return pages, nil
		}
		if err := validateNextClassCursor(reqTime, lastTime, seenCursors); err != nil {
			return nil, fmt.Errorf("invalid classlist cursor (year=%s semester=%s): %w", xnm, xqm, err)
		}

		pages = append(pages, classInfoPage{
			cursor:     reqTime,
			classInfos: append([]model.ClassInfo(nil), classInfos...),
		})
		seenCursors[lastTime] = struct{}{}
		reqTime = lastTime
	}
}

func validateNextClassCursor(current, next string, seen map[string]struct{}) error {
	if next == "" {
		return fmt.Errorf("classlist returned an empty next cursor after %q", current)
	}
	if next <= current {
		return fmt.Errorf("classlist cursor did not advance: current=%q next=%q", current, next)
	}
	if _, ok := seen[next]; ok {
		return fmt.Errorf("classlist cursor cycle detected at %q", next)
	}
	return nil
}

func unlockClassSync(locker lock.Locker, lockKey string) error {
	ok, err := locker.Unlock()
	if err != nil {
		return fmt.Errorf("failed to unlock class sync lock %s: %w", lockKey, err)
	}
	if !ok {
		return fmt.Errorf("failed to unlock class sync lock %s: lock was not released", lockKey)
	}
	return nil
}

func (c *ClassServiceUserCase) DeleteSchoolClassInfosFromES(ctx context.Context, xnm, xqm string) {
	//xnm, xqm := tool.GetXnmAndXqm()
	c.es.ClearClassInfo(ctx, xnm, xqm)
}
