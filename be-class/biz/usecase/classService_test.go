package usecase

import (
	"errors"
	"strings"
	"testing"
)

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
