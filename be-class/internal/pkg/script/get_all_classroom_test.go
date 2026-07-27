package script

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAcademicTermCode(t *testing.T) {
	got, err := academicTermCode("2025-2026", "3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2025-2026-3" {
		t.Fatalf("unexpected term code %q", got)
	}
}

func TestExtractClassroomNames(t *testing.T) {
	raw, err := json.Marshal([]any{
		[]any{},
		1,
		7,
		[]string{"一", "星期一"},
		[]any{
			[]any{" n401 ", nil, "id-1", "(48/10)", "多媒体教室"},
			[]any{"3101", nil, "id-2", "(140/50)", "多媒体教室"},
			[]any{"n401", nil, "id-1", "(48/10)", "多媒体教室"},
		},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	got, err := extractClassroomNames(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"n401", "3101"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExtractClassroomNamesRejectsMalformedRows(t *testing.T) {
	raw, err := json.Marshal([]any{
		[]any{},
		1,
		7,
		[]string{"一", "星期一"},
		[]any{
			[]any{"n401", nil, "id-1", "(48/10)", "多媒体教室"},
			[]any{map[string]string{"name": "n402"}, nil, "id-2", "(48/10)", "多媒体教室"},
		},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	if _, err := extractClassroomNames(raw); err == nil {
		t.Fatal("expected malformed classroom rows to be rejected")
	}
}
