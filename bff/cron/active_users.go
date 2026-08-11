package cron

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

const (
	activeUsersBucketMinutes = 15
	activeUsersWindowBuckets = 24 * 60 / activeUsersBucketMinutes
	activeUsersBucketTTL     = 48 * time.Hour
	activeUsersBucketPrefix  = "dau:"
)

// ActiveUsers24hRefresher counts unique users in the latest 96 completed
// 15-minute buckets. The metric is a rolling 24-hour window with 15-minute
// resolution and intentionally does not reset at midnight.
//
// Every BFF Pod refreshes its own process-local Gauge from the same Redis data.
// A distributed lock must not be used here, otherwise only one Pod is updated.
type ActiveUsers24hRefresher struct {
	redis redis.Cmdable
	gauge prometheus.Gauge
	log   logger.Logger
	temp  string
}

func NewActiveUsers24hRefresher(r redis.Cmdable, g prometheus.Gauge, l logger.Logger) *ActiveUsers24hRefresher {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	return &ActiveUsers24hRefresher{
		redis: r,
		gauge: g,
		log:   l,
		temp:  fmt.Sprintf("active_users_24h:tmp:%s:%d", hostname, os.Getpid()),
	}
}

func ActiveUsersBucketKey(t time.Time) string {
	return activeUsersBucketPrefix + t.Local().Truncate(activeUsersBucketMinutes*time.Minute).Format("2006-01-02-15-04")
}

func ActiveUsersBucketTTL() time.Duration {
	return activeUsersBucketTTL
}

func (r *ActiveUsers24hRefresher) Refresh(ctx context.Context) {
	r.refreshAt(ctx, time.Now())
}

func (r *ActiveUsers24hRefresher) refreshAt(ctx context.Context, now time.Time) {
	windowEnd := now.Local().Truncate(activeUsersBucketMinutes * time.Minute)
	keys := make([]string, 0, activeUsersWindowBuckets)
	for i := 1; i <= activeUsersWindowBuckets; i++ {
		keys = append(keys, ActiveUsersBucketKey(windowEnd.Add(-time.Duration(i)*activeUsersBucketMinutes*time.Minute)))
	}

	pipe := r.redis.Pipeline()
	pipe.PFMerge(ctx, r.temp, keys...)
	countCmd := pipe.PFCount(ctx, r.temp)
	pipe.Expire(ctx, r.temp, time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		r.log.Error("active users 24h refresh: count failed", logger.Error(err))
		return
	}

	count := countCmd.Val()
	r.gauge.Set(float64(count))
	r.log.Info("active users 24h refresh ok",
		logger.String("window_end", windowEnd.Format(time.RFC3339)),
		logger.Int64("count", count))
}
