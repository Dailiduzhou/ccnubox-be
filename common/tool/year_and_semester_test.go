package tool_test

import (
	"testing"
	"time"

	"github.com/asynccnu/ccnubox-be/common/tool"
)

func TestGetCurrentAcademicYearAndSemester(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		year     int
		semester int
	}{
		{name: "first semester", date: time.Date(2025, 10, 1, 0, 0, 0, 0, time.Local), year: 2025, semester: 1},
		{name: "first semester in January", date: time.Date(2026, 1, 15, 0, 0, 0, 0, time.Local), year: 2025, semester: 1},
		{name: "second semester", date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local), year: 2025, semester: 2},
		{name: "third semester in July", date: time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local), year: 2025, semester: 3},
		{name: "third semester in August", date: time.Date(2026, 8, 2, 0, 0, 0, 0, time.Local), year: 2025, semester: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, semester := tool.GetCurrentAcademicYearAndSemester(tt.date)
			if year != tt.year || semester != tt.semester {
				t.Fatalf("got (%d, %d), want (%d, %d)", year, semester, tt.year, tt.semester)
			}
		})
	}
}
