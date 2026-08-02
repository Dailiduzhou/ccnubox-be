package model

type AddClassRequest struct {
	StudentID string
	Name      string
	Duration  string
	Where     string
	Teacher   string
	Weeks     int64
	Semester  string
	Year      string
	Day       int64
	Credit    *float64
}

type AddClassResult struct {
	ID  string
	Msg string
}

type ToBeStudiedClasses struct {
	IdentityDevelop []ToBeStudiedClass
	SpecificSkill   []ToBeStudiedClass
	CommonEducate   []ToBeStudiedClass
}

type AvailableClassroomStat struct {
	Classroom     string
	AvailableStat []bool
}
