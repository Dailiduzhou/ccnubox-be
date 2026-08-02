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
