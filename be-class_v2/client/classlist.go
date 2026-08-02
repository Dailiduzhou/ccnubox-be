package client

import (
	"context"

	"github.com/asynccnu/ccnubox-be/be-class_v2/biz/model"
	classlistv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/classlist/v1"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
)

type ClassListService struct {
	client classlistv1.ClasserClient
	logger logger.Logger
}

func NewClassListService(client classlistv1.ClasserClient, l logger.Logger) *ClassListService {
	return &ClassListService{client: client, logger: l}
}

func (c *ClassListService) GetAllSchoolClassInfos(ctx context.Context, year, semester, cursor string) ([]model.ClassInfo, string, error) {
	resp, err := c.client.GetAllClassInfo(ctx, &classlistv1.GetAllClassInfoRequest{Year: year, Semester: semester, Cursor: cursor})
	if err != nil {
		c.logger.WithContext(ctx).Error("get all class information failed", logger.String("year", year), logger.String("semester", semester), logger.Error(err))
		return nil, "", err
	}
	classes := make([]model.ClassInfo, 0, len(resp.ClassInfos))
	for _, info := range resp.ClassInfos {
		classes = append(classes, model.ClassInfo{ID: info.Id, Day: info.Day, Teacher: info.Teacher,
			Where: info.Where, ClassWhen: info.ClassWhen, WeekDuration: info.WeekDuration,
			Classname: info.Classname, Credit: info.Credit, Weeks: info.Weeks,
			Semester: info.Semester, Year: info.Year})
	}
	return classes, resp.LastTime, nil
}

func (c *ClassListService) AddClass(ctx context.Context, req model.AddClassRequest) (model.AddClassResult, error) {
	resp, err := c.client.AddClass(ctx, &classlistv1.AddClassRequest{
		StuId: req.StudentID, Name: req.Name, DurClass: req.Duration, Where: req.Where,
		Teacher: req.Teacher, Weeks: req.Weeks, Semester: req.Semester, Year: req.Year,
		Day: req.Day, Credit: req.Credit,
	})
	if err != nil {
		c.logger.WithContext(ctx).Error("add class through classlist failed", logger.Error(err))
		return model.AddClassResult{}, err
	}
	return model.AddClassResult{ID: resp.Id, Msg: resp.Msg}, nil
}

func (c *ClassListService) GetSchoolDay(ctx context.Context) (string, error) {
	resp, err := c.client.GetSchoolDay(ctx, &classlistv1.GetSchoolDayReq{})
	if err != nil {
		return "", err
	}
	return resp.SchoolTime, nil
}
