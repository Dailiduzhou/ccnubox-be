package metricsx

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
)

type pipelineCmdable struct {
	redis.Cmdable
	pipeline   redis.Pipeliner
	txPipeline redis.Pipeliner
}

func (c pipelineCmdable) Pipeline() redis.Pipeliner {
	return c.pipeline
}

func (c pipelineCmdable) TxPipeline() redis.Pipeliner {
	return c.txPipeline
}

type execPipeliner struct {
	redis.Pipeliner
	err error
}

func (p execPipeliner) Exec(context.Context) ([]redis.Cmder, error) {
	return nil, p.err
}

func TestInstrumentedRedisPipeline(t *testing.T) {
	tests := []struct {
		name          string
		transactional bool
		err           error
		operation     string
		status        string
		errorType     string
	}{
		{
			name:      "successful pipeline",
			operation: "PIPELINE",
			status:    "OK",
		},
		{
			name:          "failed transaction pipeline",
			transactional: true,
			err:           context.DeadlineExceeded,
			operation:     "TXPIPELINE",
			status:        "Error",
			errorType:     "timeout",
		},
		{
			name:      "missing key is not an infrastructure error",
			err:       redis.Nil,
			operation: "PIPELINE",
			status:    "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			metrics := NewWithRegisterer(registry, "test")
			underlying := execPipeliner{err: tt.err}
			client := NewInstrumentedRedis(pipelineCmdable{
				pipeline:   underlying,
				txPipeline: underlying,
			}, metrics)

			pipeline := client.Pipeline()
			if tt.transactional {
				pipeline = client.TxPipeline()
			}
			_, err := pipeline.Exec(context.Background())
			if !errors.Is(err, tt.err) {
				t.Fatalf("Exec() error = %v, want %v", err, tt.err)
			}

			if got := testutil.ToFloat64(metrics.Redis.RequestsTotal.WithLabelValues(tt.operation, tt.status)); got != 1 {
				t.Fatalf("request metric = %v, want 1", got)
			}
			if tt.errorType != "" {
				if got := testutil.ToFloat64(metrics.Redis.ErrorsTotal.WithLabelValues(tt.operation, tt.errorType)); got != 1 {
					t.Fatalf("error metric = %v, want 1", got)
				}
			}
		})
	}
}

func TestInstrumentedRedisPipelined(t *testing.T) {
	tests := []struct {
		name          string
		transactional bool
		err           error
		operation     string
		status        string
		errorType     string
	}{
		{
			name:      "successful pipeline",
			operation: "PIPELINE",
			status:    "OK",
		},
		{
			name:          "failed transaction pipeline",
			transactional: true,
			err:           context.DeadlineExceeded,
			operation:     "TXPIPELINE",
			status:        "Error",
			errorType:     "timeout",
		},
		{
			name:      "missing key is not an infrastructure error",
			err:       redis.Nil,
			operation: "PIPELINE",
			status:    "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			metrics := NewWithRegisterer(registry, "test")
			underlying := execPipeliner{err: tt.err}
			client := NewInstrumentedRedis(pipelineCmdable{
				pipeline:   underlying,
				txPipeline: underlying,
			}, metrics)

			var err error
			if tt.transactional {
				_, err = client.TxPipelined(context.Background(), func(redis.Pipeliner) error {
					return nil
				})
			} else {
				_, err = client.Pipelined(context.Background(), func(redis.Pipeliner) error {
					return nil
				})
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("Pipelined() error = %v, want %v", err, tt.err)
			}

			if got := testutil.ToFloat64(metrics.Redis.RequestsTotal.WithLabelValues(tt.operation, tt.status)); got != 1 {
				t.Fatalf("request metric = %v, want 1", got)
			}
			if tt.errorType != "" {
				if got := testutil.ToFloat64(metrics.Redis.ErrorsTotal.WithLabelValues(tt.operation, tt.errorType)); got != 1 {
					t.Fatalf("error metric = %v, want 1", got)
				}
			}
		})
	}
}
