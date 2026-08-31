package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/robfig/cron/v3"
)

type ReminderScheduler struct {
	service *ReminderService
	cron    *cron.Cron
	logger  logger.Logger
	cancel  context.CancelFunc
	rootCtx context.Context
	wg      sync.WaitGroup
	mu      sync.Mutex
}

type reminderCronLogger struct{ logger logger.Logger }

func (l reminderCronLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, logger.Any("cron", keysAndValues))
}

func (l reminderCronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.logger.Error(msg, logger.Error(err), logger.Any("cron", keysAndValues))
}

func NewReminderScheduler(service *ReminderService, l logger.Logger) *ReminderScheduler {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	cronLogger := reminderCronLogger{logger: l}
	c := cron.New(cron.WithLocation(loc), cron.WithChain(cron.Recover(cronLogger), cron.SkipIfStillRunning(cronLogger)))
	scheduler := &ReminderScheduler{service: service, cron: c, logger: l}
	if !service.Enabled() {
		return scheduler
	}
	for _, entry := range []struct {
		spec string
		fn   func(context.Context) error
	}{
		{service.config.FullRefreshCron, service.RefreshAll},
		{service.config.PreferenceFullSyncCron, service.CalibratePreferences},
		{service.config.ActiveScanCron, service.ScanActive},
		{service.config.JobDispatchCron, service.DispatchJobs},
	} {
		fn := entry.fn
		if _, err := c.AddFunc(entry.spec, func() { scheduler.run("cron", fn) }); err != nil {
			panic(fmt.Sprintf("invalid library reminder cron %q: %v", entry.spec, err))
		}
	}
	return scheduler
}

func (s *ReminderScheduler) Start() error {
	if !s.service.Enabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.service.SuppressDisabledWork(ctx)
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil
	}
	recoverCtx, recoverCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer recoverCancel()
	err := s.service.RecoverOrphanedWork(recoverCtx)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.rootCtx = ctx
	s.mu.Unlock()
	s.cron.Start()
	s.startLoop(ctx, "preference_sync", s.service.config.PreferenceSyncInterval, s.service.SyncPreferences, true)
	s.startLoop(ctx, "outbox", s.service.config.OutboxInterval, s.service.SendOutbox, false)
	return nil
}

func (s *ReminderScheduler) startLoop(ctx context.Context, name string, interval time.Duration, fn func(context.Context) error, immediate bool) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if immediate {
			s.runWithContext(ctx, name, fn)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runWithContext(ctx, name, fn)
			}
		}
	}()
}

func (s *ReminderScheduler) run(name string, fn func(context.Context) error) {
	s.mu.Lock()
	cancel := s.cancel
	ctx := s.rootCtx
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	// Cron 不暴露根上下文；为每次操作设置超时上下文，确保关闭时可以等待且不会泄漏上游请求。
	taskCtx, stop := context.WithTimeout(ctx, 25*time.Minute)
	defer stop()
	s.runWithContext(taskCtx, name, fn)
}

func (s *ReminderScheduler) runWithContext(ctx context.Context, name string, fn func(context.Context) error) {
	if err := fn(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("library reminder task failed", logger.String("task", name), logger.Error(err))
	}
}

func (s *ReminderScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel == nil {
		s.mu.Unlock()
		return nil
	}
	s.cancel()
	s.cancel = nil
	s.rootCtx = nil
	s.mu.Unlock()
	cronCtx := s.cron.Stop()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-cronCtx.Done():
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
