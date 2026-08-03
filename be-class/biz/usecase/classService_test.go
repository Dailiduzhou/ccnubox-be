package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/asynccnu/ccnubox-be/be-class/biz/model"
)

const initialClassCursor = "1949-10-01T00:00:00.000000"

type invalidSecondPageClassList struct{}

func (invalidSecondPageClassList) GetAllSchoolClassInfos(_ context.Context, _, _, cursor string) ([]model.ClassInfo, string, error) {
	switch cursor {
	case initialClassCursor:
		return []model.ClassInfo{{ID: "first-page"}}, "2026-01-02T00:00:00.000000", nil
	case "2026-01-02T00:00:00.000000":
		return []model.ClassInfo{{ID: "second-page"}}, cursor, nil
	default:
		return nil, "", errors.New("unexpected cursor")
	}
}

func (invalidSecondPageClassList) AddClass(context.Context, model.AddClassRequest) (model.AddClassResult, error) {
	return model.AddClassResult{}, nil
}

type recordingEsProxy struct {
	addCalls int
}

func (r *recordingEsProxy) AddClassInfo(context.Context, ...model.ClassInfo) error {
	r.addCalls++
	return nil
}

func (*recordingEsProxy) ClearClassInfo(context.Context, string, string) {}

func (*recordingEsProxy) SearchClassInfo(context.Context, string, string, string, int, int) ([]model.ClassInfo, error) {
	return nil, nil
}

type fakeClassSyncLocker struct {
	ok  bool
	err error
}

func (f fakeClassSyncLocker) Lock() error {
	return nil
}

func (f fakeClassSyncLocker) Unlock() (bool, error) {
	return f.ok, f.err
}

func TestUnlockClassSync(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		if err := unlockClassSync(fakeClassSyncLocker{ok: true}, "lock-key"); err != nil {
			t.Fatalf("expected unlock to succeed: %v", err)
		}
	})

	t.Run("unlock error", func(t *testing.T) {
		wantErr := errors.New("redis unavailable")
		err := unlockClassSync(fakeClassSyncLocker{err: wantErr}, "lock-key")
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected wrapped unlock error, got %v", err)
		}
	})

	t.Run("lock not released", func(t *testing.T) {
		err := unlockClassSync(fakeClassSyncLocker{}, "lock-key")
		if err == nil || !strings.Contains(err.Error(), "lock was not released") {
			t.Fatalf("expected lock-not-released error, got %v", err)
		}
	})
}

func TestValidateNextClassCursor(t *testing.T) {
	seen := map[string]struct{}{"2026-01-01T00:00:00.000000": {}}
	if err := validateNextClassCursor("2026-01-01T00:00:00.000000", "2026-01-02T00:00:00.000000", seen); err != nil {
		t.Fatalf("expected advancing cursor to pass: %v", err)
	}

	for _, next := range []string{"", "2026-01-01T00:00:00.000000", "2025-12-31T23:59:59.000000"} {
		if err := validateNextClassCursor("2026-01-01T00:00:00.000000", next, seen); err == nil {
			t.Fatalf("expected cursor %q to be rejected", next)
		}
	}
}

func TestAddClassInfosToESDoesNotPublishWhenSecondPageCursorIsInvalid(t *testing.T) {
	es := &recordingEsProxy{}
	service := &ClassServiceUserCase{
		es: es,
		cs: invalidSecondPageClassList{},
	}

	err := service.AddClassInfosToES(context.Background(), "2025", "1")
	if err == nil || !strings.Contains(err.Error(), "cursor did not advance") {
		t.Fatalf("expected invalid second-page cursor error, got %v", err)
	}
	if es.addCalls != 0 {
		t.Fatalf("expected no Elasticsearch writes before all cursors are validated, got %d", es.addCalls)
	}
}
