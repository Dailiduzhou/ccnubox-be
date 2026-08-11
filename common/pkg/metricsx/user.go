package metricsx

import "github.com/prometheus/client_golang/prometheus"

// UserMetrics 用户行为相关指标。
// ActiveUsers24h 是最近 96 个已完成的 15 分钟桶内的去重活跃用户数。
type UserMetrics struct {
	ActiveUsers24h prometheus.Gauge
}

func newUserMetrics(namespace string) *UserMetrics {
	return &UserMetrics{
		ActiveUsers24h: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, "", "active_users_24h"),
			Help: "Unique active users over the latest 96 completed 15-minute buckets (rolling 24 hours).",
		}),
	}
}
