package dao

import (
	"slices"
	"testing"
)

func TestGradeUpdateColumns(t *testing.T) {
	base := gradeUpdateColumns(false)
	if slices.Contains(base, "regular_grade") {
		t.Fatal("base grade update unexpectedly overwrites detail columns")
	}

	detail := gradeUpdateColumns(true)
	for _, column := range []string{"regular_grade_percent", "regular_grade", "final_grade_percent", "final_grade"} {
		if !slices.Contains(detail, column) {
			t.Fatalf("detail grade update is missing %q", column)
		}
	}
}
