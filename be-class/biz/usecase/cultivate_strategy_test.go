package usecase

import "testing"

func TestBetterStudiableRejectsInvalidStudentID(t *testing.T) {
	const term = "2024-2025-1"
	for _, studentID := range []string{"", "123", "abcd2024"} {
		if got := betterStudiable(term, studentID); got != term {
			t.Fatalf("betterStudiable(%q, %q) = %q, want original term", term, studentID, got)
		}
	}
}
