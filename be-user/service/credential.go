package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	ccnuv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/ccnu/v1"
	"github.com/asynccnu/ccnubox-be/common/tool"
)

const (
	credentialTimeout   = 30 * time.Second
	maxCredentialLogins = 8
)

// 单实例内按学号串行化凭证读写与账号变更；扩容为多实例前需改为跨进程版本校验。
type credentialGuard struct {
	mu      sync.Mutex
	entries map[string]*credentialLock
	once    sync.Once
	slots   chan struct{}
}

type credentialLock struct {
	token chan struct{}
	refs  int
}

func (g *credentialGuard) lock(ctx context.Context, sid string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	if g.entries == nil {
		g.entries = make(map[string]*credentialLock)
	}
	entry := g.entries[sid]
	if entry == nil {
		entry = &credentialLock{token: make(chan struct{}, 1)}
		g.entries[sid] = entry
	}
	entry.refs++
	g.mu.Unlock()

	release := func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		entry.refs--
		if entry.refs == 0 {
			delete(g.entries, sid)
		}
	}
	select {
	case entry.token <- struct{}{}:
		unlock := func() {
			<-entry.token
			release()
		}
		if err := ctx.Err(); err != nil {
			unlock()
			return nil, err
		}
		return unlock, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}

// 冷缓存及 Redis 故障仍允许登录降级，但不同账号的校方登录总并发有界。
func (g *credentialGuard) acquireLogin(ctx context.Context) (func(), error) {
	g.once.Do(func() { g.slots = make(chan struct{}, maxCredentialLogins) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case g.slots <- struct{}{}:
		return func() { <-g.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// 凭证链路只在用户服务重试，最多三次；退避和排队也计入任务总超时。
func retryCredential[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var result T
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result, err = fn()
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if attempt == 2 || ccnuv1.IsInvalidSidOrPwd(err) || ccnuv1.IsInvalidXkSystem(err) || tool.IsCCNUAccountInitializationRequired(err) {
			return result, err
		}
		if attempt > 0 {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return result, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return result, err
}

func (s *userService) forgetCredentials(sid string) {
	for _, system := range []tool.XKSystem{tool.XKUndergraduate, tool.XKPostgraduate} {
		s.sfGroup.Forget(fmt.Sprintf("xk:%s:%s", system, sid))
	}
	for _, serviceType := range []ccnuv1.LIBRARY_TYPE{ccnuv1.LIBRARY_TYPE_LIBRARY_SEAT, ccnuv1.LIBRARY_TYPE_LIBRARY_DISCUSSION} {
		s.sfGroup.Forget(fmt.Sprintf("lib:%s:%s", serviceType, sid))
	}
}
