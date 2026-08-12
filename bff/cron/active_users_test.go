package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{
		Addr:        mr.Addr(),
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	t.Cleanup(func() { _ = client.Close() })
	return mr, client
}

func newTestGauge() prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_active_users_24h", Help: "test"})
}

type nopLogger struct{}

func (nopLogger) WithContext(context.Context) logger.Logger { return nopLogger{} }
func (nopLogger) With(...logger.Field) logger.Logger        { return nopLogger{} }
func (nopLogger) Debug(string, ...logger.Field)             {}
func (nopLogger) Info(string, ...logger.Field)              {}
func (nopLogger) Warn(string, ...logger.Field)              {}
func (nopLogger) Error(string, ...logger.Field)             {}
func (nopLogger) Debugf(string, ...interface{})             {}
func (nopLogger) Infof(string, ...interface{})              {}
func (nopLogger) Warnf(string, ...interface{})              {}
func (nopLogger) Errorf(string, ...interface{})             {}
func (nopLogger) AddCallerSkip(int) logger.Logger           { return nopLogger{} }

var _ logger.Logger = nopLogger{}

func readGauge(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(g); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegistered) {
			t.Fatalf("register gauge: %v", err)
		}
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

func TestActiveUsers24hRefresherCountsRollingWindow(t *testing.T) {
	mr, client := newTestRedis(t)
	gauge := newTestGauge()
	refresher := NewActiveUsers24hRefresher(client, gauge, nopLogger{})
	now := time.Date(2026, 6, 16, 10, 15, 0, 0, time.Local)

	// Both boundaries are included: [previous day 10:15, today 10:15).
	mr.PfAdd(ActiveUsersBucketKey(now.Add(-24*time.Hour)), "stu-1")
	mr.PfAdd(ActiveUsersBucketKey(now.Add(-15*time.Minute)), "stu-1", "stu-2")
	// The current partial bucket and the bucket older than 24h are excluded.
	mr.PfAdd(ActiveUsersBucketKey(now), "stu-current")
	mr.PfAdd(ActiveUsersBucketKey(now.Add(-24*time.Hour-15*time.Minute)), "stu-old")

	refresher.refreshAt(context.Background(), now)

	if got := readGauge(t, gauge); got != 2 {
		t.Fatalf("gauge got %v, want 2", got)
	}
}

func TestActiveUsers24hRefresherCrossesMidnight(t *testing.T) {
	mr, client := newTestRedis(t)
	gauge := newTestGauge()
	refresher := NewActiveUsers24hRefresher(client, gauge, nopLogger{})
	now := time.Date(2026, 6, 17, 0, 0, 0, 0, time.Local)

	mr.PfAdd(ActiveUsersBucketKey(now.Add(-15*time.Minute)), "late-user")
	mr.PfAdd(ActiveUsersBucketKey(now.Add(-23*time.Hour)), "morning-user")

	refresher.refreshAt(context.Background(), now)

	if got := readGauge(t, gauge); got != 2 {
		t.Fatalf("gauge reset across midnight: got %v, want 2", got)
	}
}

func TestActiveUsers24hRefresherKeepsLastValueOnRedisError(t *testing.T) {
	mr, client := newTestRedis(t)
	gauge := newTestGauge()
	gauge.Set(42)
	refresher := NewActiveUsers24hRefresher(client, gauge, nopLogger{})
	mr.Close()

	refresher.refreshAt(context.Background(), time.Now())

	if got := readGauge(t, gauge); got != 42 {
		t.Fatalf("gauge got %v after Redis error, want previous value 42", got)
	}
}
