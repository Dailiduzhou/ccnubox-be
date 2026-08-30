package service

import (
	"context"
	"errors"
	"testing"

	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger/zapx"
	"go.uber.org/zap"
)

func testLogger() logger.Logger {
	return zapx.NewZapLogger(zap.NewNop())
}

type fakeContentClient struct {
	year, semester string
	err            error
}

func (f *fakeContentClient) GetCurrentSemester(ctx context.Context) (string, string, error) {
	return f.year, f.semester, f.err
}

func TestIsCurrentSemester(t *testing.T) {
	tests := []struct {
		name     string
		curYear  string
		curSem   string
		year     string
		semester string
		want     bool
	}{
		{name: "当前学期", curYear: "2026", curSem: "1", year: "2026", semester: "1", want: true},
		{name: "下一学期被拒", curYear: "2026", curSem: "1", year: "2026", semester: "2", want: false},
		{name: "上一学年同学期被拒", curYear: "2026", curSem: "1", year: "2025", semester: "1", want: false},
		{name: "非法目标学年", curYear: "2026", curSem: "1", year: "abc", semester: "1", want: false},
		{name: "非法目标学期", curYear: "2026", curSem: "1", year: "2026", semester: "abc", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCurrentSemester(tt.curYear, tt.curSem, tt.year, tt.semester); got != tt.want {
				t.Errorf("isCurrentSemester(%q,%q,%q,%q) = %v, want %v", tt.curYear, tt.curSem, tt.year, tt.semester, got, tt.want)
			}
		})
	}
}

func TestCheckCurrentSemester(t *testing.T) {
	t.Run("当前学期通过", func(t *testing.T) {
		s := NewClasserService(nil, nil, &fakeContentClient{year: "2026", semester: "1"}, testLogger())
		ok, err := s.checkCurrentSemester(context.Background(), "2026", "1")
		if err != nil || !ok {
			t.Errorf("当前学期应通过, ok=%v err=%v", ok, err)
		}
	})

	t.Run("非当前学期被拒", func(t *testing.T) {
		s := NewClasserService(nil, nil, &fakeContentClient{year: "2026", semester: "1"}, testLogger())
		ok, err := s.checkCurrentSemester(context.Background(), "2026", "2")
		if err != nil || ok {
			t.Errorf("非当前学期应被拒, ok=%v err=%v", ok, err)
		}
	})

	t.Run("be-content 不可用返回错误", func(t *testing.T) {
		s := NewClasserService(nil, nil, &fakeContentClient{err: errors.New("content down")}, testLogger())
		ok, err := s.checkCurrentSemester(context.Background(), "2026", "1")
		if err == nil || ok {
			t.Errorf("be-content 不可用应返回错误, ok=%v err=%v", ok, err)
		}
	})
}
