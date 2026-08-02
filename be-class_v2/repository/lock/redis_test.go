package lock

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestRedisLock(t *testing.T) {
	server := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{
		Addr: server.Addr(),
	})
	t.Cleanup(func() { _ = cli.Close() })
	builder := NewRedisLockBuilder(cli)
	locker1 := builder.Build("test")
	locker2 := builder.Build("test")

	t.Run("the lock is locked", func(t *testing.T) {
		err := locker1.Lock()
		assert.NoError(t, err)
		err = locker2.Lock()
		assert.Error(t, err)
		ok, err := locker1.Unlock()
		assert.NoError(t, err)
		assert.True(t, ok)
	})
	t.Run("the lock is unlocked", func(t *testing.T) {
		err := locker1.Lock()
		assert.NoError(t, err)
		ok, err := locker1.Unlock()
		assert.NoError(t, err)
		assert.True(t, ok)
		err = locker2.Lock()
		assert.NoError(t, err)
		ok, err = locker2.Unlock()
		assert.NoError(t, err)
		assert.True(t, ok)
	})
}
