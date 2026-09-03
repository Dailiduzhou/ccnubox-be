package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/asynccnu/ccnubox-be/be-library/tool"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/robfig/cron/v3"
)

const reminderTaskTimeout = 25 * time.Minute

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
	loc := tool.GetLocation()
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
	// 瞬时数据库故障导致释放失败时，无需等待服务重启即可恢复超时 claim。
	s.startLoop(ctx, "claim_recovery", s.service.config.ClaimRecoveryInterval, s.service.RecoverStaleWork, false)
	// 终态记录的低频清理，避免每两秒的 outbox 聚合扫描无限增长的历史表。
	s.startLoop(ctx, "history_cleanup", time.Hour, s.service.CleanupHistory, true)
	return nil
}

func (s *ReminderScheduler) startLoop(ctx context.Context, name string, interval time.Duration, fn func(context.Context) error, immediate bool) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if immediate {
			s.runTask(ctx, name, fn)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runTask(ctx, name, fn)
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
	s.runTask(ctx, name, fn)
}

// runTask 为 cron 和固定间隔任务统一设置单次执行边界。
// 超时依赖下游遵守 context，无法强制终止忽略取消信号的调用。
func (s *ReminderScheduler) runTask(parent context.Context, name string, fn func(context.Context) error) {
	taskCtx, cancel := context.WithTimeout(parent, reminderTaskTimeout)
	defer cancel()
	if err := fn(taskCtx); err != nil && parent.Err() == nil {
		s.logger.Warn("library reminder task failed", logger.String("task", name), logger.Error(err))
	}
}

func (s *ReminderScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel == nil {
		s.mu.Unlock()
		return nil
	}
	// 先取消所有任务，但在 cron 与固定间隔 loop 全部退出前保留运行状态；
	// 若本次等待超时，后续 Stop 仍可继续等待，避免误判为已经安全停止。
	s.cancel()
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
	}
	s.mu.Lock()
	s.cancel = nil
	s.rootCtx = nil
	s.mu.Unlock()
	return nil
}
