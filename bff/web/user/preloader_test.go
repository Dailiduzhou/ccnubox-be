package user

import (
	"context"
	"testing"
	"time"
)

func TestAcademicYear(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{
			name: "before September",
			now:  time.Date(2026, time.August, 31, 23, 59, 59, 0, time.Local),
			want: "2025",
		},
		{
			name: "from September",
			now:  time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local),
			want: "2026",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := academicYear(tt.now); got != tt.want {
				t.Fatalf("academicYear(%v) = %q, want %q", tt.now, got, tt.want)
			}
		})
	}
}

func TestRunPreloadOutlivesParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	result := make(chan error, 1)
	runPreload(parent, func(ctx context.Context) {
		result <- ctx.Err()
	})

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("preload context was canceled with parent: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("preload task did not run")
	}
}
