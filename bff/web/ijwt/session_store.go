package ijwt

import (
	"context"
	"errors"
	"strings"
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

var _ SessionStore = (*RedisSessionStore)(nil)

func NewRedisSessionStore(cmd redis.Cmdable) *RedisSessionStore {
	return &RedisSessionStore{cmd: cmd}
}

func sessionKey(ssid string) (string, error) {
	if strings.TrimSpace(ssid) == "" {
		return "", errors.New("session id is required")
	}
	return "ccnubox:users:ssid:" + ssid, nil
}

func (s *RedisSessionStore) Revoke(ctx context.Context, ssid string, ttl time.Duration) error {
	key, err := sessionKey(ssid)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return errors.New("session revocation TTL must be positive")
	}
	return s.cmd.Set(ctx, key, "", ttl).Err()
}

func (s *RedisSessionStore) IsRevoked(ctx context.Context, ssid string) (bool, error) {
	key, err := sessionKey(ssid)
	if err != nil {
		return false, err
	}
	n, err := s.cmd.Exists(ctx, key).Result()
	return n > 0, err
}
