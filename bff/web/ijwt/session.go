package ijwt

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type SessionStore interface {
	Revoke(ctx context.Context, ssid string, ttl time.Duration) error
	IsRevoked(ctx context.Context, ssid string) (bool, error)
}

type RedisSessionStore struct {
	cmd redis.Cmdable
}

func NewRedisSessionStore(cmd redis.Cmdable) *RedisSessionStore {
	return &RedisSessionStore{cmd: cmd}
}

func (s *RedisSessionStore) Revoke(ctx context.Context, ssid string, ttl time.Duration) error {
	return s.cmd.Set(ctx, s.key(ssid), "", ttl).Err()
}

func (s *RedisSessionStore) IsRevoked(ctx context.Context, ssid string) (bool, error) {
	val, err := s.cmd.Exists(ctx, s.key(ssid)).Result()
	return val > 0, err
}

func (s *RedisSessionStore) key(ssid string) string {
	return fmt.Sprintf("ccnubox:users:ssid:%s", ssid)
}
