package ioc

import (
	"context"

	"github.com/asynccnu/ccnubox-be/bff/cron"
	"github.com/asynccnu/ccnubox-be/common/pkg/cronx"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"github.com/redis/go-redis/v9"
)

const activeUsers24hCronSpec = "*/15 * * * *"

func InitCronxManager(
	l logger.Logger,
	m *metricsx.Metrics,
	redisClient redis.Cmdable,
) *cronx.Manager {
	manager := cronx.NewManager(l)
	registerActiveUsers24hRefreshTask(manager, m, redisClient, l)
	return manager
}

func registerActiveUsers24hRefreshTask(
	cronMgr *cronx.Manager,
	m *metricsx.Metrics,
	redisClient redis.Cmdable,
	l logger.Logger,
) {
	refresher := cron.NewActiveUsers24hRefresher(redisClient, m.User.ActiveUsers24h, l)
	refresher.Refresh(context.Background())

	if err := cronMgr.AddTask("active_users_24h_refresh", activeUsers24hCronSpec, func(ctx context.Context, log logger.Logger) {
		refresher.Refresh(ctx)
	}); err != nil {
		l.Warn("active users 24h refresh: register cron task failed", logger.Error(err))
	}
}
