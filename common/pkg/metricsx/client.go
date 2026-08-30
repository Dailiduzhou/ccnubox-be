package metricsx

import "github.com/prometheus/client_golang/prometheus"

type ClientMetrics struct {
	AppErrorsTotal      *prometheus.CounterVec
	APIFailuresTotal    *prometheus.CounterVec
	StartupDuration     *prometheus.HistogramVec
	IngestedEventsTotal *prometheus.CounterVec
	RejectedBatches     *prometheus.CounterVec
}

func newClientMetrics(namespace string) *ClientMetrics {
	return &ClientMetrics{
		AppErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "app_error_total",
				Help:      "Total application errors reported by mobile clients.",
			},
			[]string{"platform", "app_version", "module", "level"},
		),
		APIFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "app_api_failure_total",
				Help:      "Total API failures reported by mobile clients.",
			},
			[]string{"platform", "api_group", "status_code"},
		),
		StartupDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "app_startup_duration_seconds",
				Help:      "Mobile application startup duration in seconds.",
				Buckets:   []float64{0.25, 0.5, 1, 2, 3, 5, 8, 13, 20, 30},
			},
			[]string{"platform", "app_version"},
		),
		IngestedEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "client_metric_events_total",
				Help:      "Total accepted client metric events.",
			},
			[]string{"event"},
		),
		RejectedBatches: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "client_metric_rejected_batches_total",
				Help:      "Total rejected client metric batches.",
			},
			[]string{"reason"},
		),
	}
}
