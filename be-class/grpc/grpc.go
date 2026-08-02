package grpc

import (
	"context"

	"github.com/asynccnu/ccnubox-be/be-class/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class/service"
	classv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/classService/v1"
	"github.com/google/wire"
	"google.golang.org/grpc"
)

var ProviderSet = wire.NewSet(NewClassServer)

type ClassServer struct {
	classv1.UnimplementedClassServiceServer
	classv1.UnimplementedFreeClassroomSvcServer
	class *service.ClassService
	free  *service.FreeClassroomService
}

func NewClassServer(class *service.ClassService, free *service.FreeClassroomService) *ClassServer {
	return &ClassServer{class: class, free: free}
}

func (s *ClassServer) Register(server grpc.ServiceRegistrar) {
	classv1.RegisterClassServiceServer(server, s)
	classv1.RegisterFreeClassroomSvcServer(server, s)
}

func (s *ClassServer) SearchClass(ctx context.Context, req *classv1.SearchRequest) (*classv1.SearchReply, error) {
	classes, err := s.class.SearchClass(ctx, req.GetSearchKeyWords(), req.GetYear(), req.GetSemester(), int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, err
	}
	result := make([]*classv1.ClassInfo, 0, len(classes))
	for _, class := range classes {
		result = append(result, classInfoToProto(class))
	}
	return &classv1.SearchReply{ClassInfos: result}, nil
}

func (s *ClassServer) AddClass(ctx context.Context, req *classv1.AddClassRequest) (*classv1.AddClassReply, error) {
	result, err := s.class.AddClass(ctx, model.AddClassRequest{
		StudentID: req.GetStuId(), Name: req.GetName(), Duration: req.GetDurClass(),
		Where: req.GetWhere(), Teacher: req.GetTeacher(), Weeks: req.GetWeeks(),
		Semester: req.GetSemester(), Year: req.GetYear(), Day: req.GetDay(), Credit: req.Credit,
	})
	if err != nil {
		return nil, err
	}
	return &classv1.AddClassReply{Id: result.ID, Msg: result.Msg}, nil
}

func (s *ClassServer) GetClassToBeStudied(ctx context.Context, req *classv1.GetClassToBeStudiedRequest) (*classv1.GetClassToBeStudiedReply, error) {
	classes, err := s.class.GetClassToBeStudied(ctx, req.GetStuId(), req.GetStatus())
	if err != nil {
		return nil, err
	}
	return &classv1.GetClassToBeStudiedReply{
		IdentityDevelop: studiedClassesToProto(classes.IdentityDevelop),
		SpecificSkill:   studiedClassesToProto(classes.SpecificSkill),
		CommonEducate:   studiedClassesToProto(classes.CommonEducate),
	}, nil
}

func (s *ClassServer) QueryFreeClassroom(ctx context.Context, req *classv1.QueryFreeClassroomReq) (*classv1.QueryFreeClassroomResp, error) {
	sections := make([]int, len(req.GetSections()))
	for i, section := range req.GetSections() {
		sections[i] = int(section)
	}
	stats, err := s.free.Query(ctx, req.GetYear(), req.GetSemester(), req.GetStuID(), int(req.GetWeek()), int(req.GetDay()), sections, req.GetWherePrefix())
	if err != nil {
		return nil, err
	}
	result := make([]*classv1.ClassroomAvailableStat, 0, len(stats))
	for _, stat := range stats {
		result = append(result, &classv1.ClassroomAvailableStat{Classroom: stat.Classroom, AvailableStat: stat.AvailableStat})
	}
	return &classv1.QueryFreeClassroomResp{Stat: result}, nil
}

func (s *ClassServer) GetClassrooms(context.Context, *classv1.GetClassroomsReq) (*classv1.GetClassroomsResp, error) {
	classrooms, err := s.free.GetClassrooms()
	if err != nil {
		return nil, err
	}
	return &classv1.GetClassroomsResp{ClassRooms: classrooms}, nil
}

func classInfoToProto(class model.ClassInfo) *classv1.ClassInfo {
	return &classv1.ClassInfo{Day: class.Day, Teacher: class.Teacher, Where: class.Where,
		ClassWhen: class.ClassWhen, WeekDuration: class.WeekDuration, Classname: class.Classname,
		Credit: class.Credit, Weeks: class.Weeks, Semester: class.Semester, Year: class.Year, Id: class.ID}
}

func studiedClassesToProto(classes []model.ToBeStudiedClass) []*classv1.GetClassToBeStudiedReply_ClassToBeStudiedInfo {
	result := make([]*classv1.GetClassToBeStudiedReply_ClassToBeStudiedInfo, 0, len(classes))
	for _, class := range classes {
		result = append(result, &classv1.GetClassToBeStudiedReply_ClassToBeStudiedInfo{
			Id: class.Id, Name: class.Name, Property: class.Property, Status: class.Status,
			Credit: class.Credit, Studiable: class.Studiable,
		})
	}
	return result
}
