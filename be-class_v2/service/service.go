package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/asynccnu/ccnubox-be/be-class_v2/biz"
	"github.com/asynccnu/ccnubox-be/be-class_v2/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class_v2/biz/usecase"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewClassService, NewFreeClassroomService)

type ClassService struct {
	class     *usecase.ClassServiceUserCase
	cultivate *usecase.CultivateStrategyBiz
}

func NewClassService(class *usecase.ClassServiceUserCase, cultivate *usecase.CultivateStrategyBiz) *ClassService {
	return &ClassService{class: class, cultivate: cultivate}
}

func (s *ClassService) SearchClass(ctx context.Context, keywords, year, semester string, page, pageSize int) ([]model.ClassInfo, error) {
	if page <= 0 || pageSize <= 0 {
		return nil, errors.New("page and pageSize must be greater than 0")
	}
	return s.class.SearchClassInfo(ctx, keywords, year, semester, page, pageSize)
}

func (s *ClassService) AddClass(ctx context.Context, req model.AddClassRequest) (model.AddClassResult, error) {
	return s.class.AddClassInfoToClassListService(ctx, req)
}

func (s *ClassService) GetClassToBeStudied(ctx context.Context, studentID, status string) (model.ToBeStudiedClasses, error) {
	return s.cultivate.GetToBeStudiedClass(ctx, studentID, status)
}

type ClassroomJSONProvider interface{ ClassroomJSON() []byte }

type FreeClassroomService struct {
	searcher *usecase.FreeClassroomBiz
	provider ClassroomJSONProvider
}

func NewFreeClassroomService(searcher *usecase.FreeClassroomBiz, provider ClassroomJSONProvider) *FreeClassroomService {
	return &FreeClassroomService{searcher: searcher, provider: provider}
}

func (s *FreeClassroomService) Query(ctx context.Context, year, semester, studentID string, week, day int, sections []int, wherePrefix string) ([]model.AvailableClassroomStat, error) {
	stats, err := s.searcher.SearchAvailableClassroom(ctx, year, semester, studentID, week, day, sections, wherePrefix)
	if err != nil {
		return nil, biz.ErrFreeClassroomSearch
	}
	return stats, nil
}

func (s *FreeClassroomService) GetClassrooms() ([]string, error) {
	var data struct {
		ClassRooms []string `json:"class_rooms"`
	}
	if err := json.Unmarshal(s.provider.ClassroomJSON(), &data); err != nil {
		return nil, biz.ErrFreeClassroomSearch
	}
	return data.ClassRooms, nil
}
