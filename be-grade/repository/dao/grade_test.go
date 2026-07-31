package dao

import (
	"slices"
	"testing"
)

func TestGradeUpdateColumns(t *testing.T) {
	baseWant := []string{
		"kc_id", "kcmc", "xnm", "xqm", "xf", "kcxzmc", "kclbmc", "kcbj", "jd", "cj",
	}
	base := gradeUpdateColumns(false)
	if !slices.Equal(base, baseWant) {
		t.Fatalf("gradeUpdateColumns(false) = %v, want %v", base, baseWant)
	}

	detailWant := append(slices.Clone(baseWant),
		"regular_grade_percent", "regular_grade", "final_grade_percent", "final_grade",
	)
	detail := gradeUpdateColumns(true)
	if !slices.Equal(detail, detailWant) {
		t.Fatalf("gradeUpdateColumns(true) = %v, want %v", detail, detailWant)
	}
}
