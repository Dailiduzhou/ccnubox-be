package metrics

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

const testClientKey = "test-client-key"

func TestClientMetricsRequiresConfiguredBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		options    ClientOptions
		authHeader string
		wantStatus int
		reason     string
	}{
		{
			name:       "collector disabled when key is empty",
			options:    ClientOptions{},
			authHeader: "Bearer anything",
			wantStatus: http.StatusServiceUnavailable,
			reason:     "collector_disabled",
		},
		{
			name:       "missing bearer token",
			options:    ClientOptions{ClientKey: testClientKey},
			wantStatus: http.StatusUnauthorized,
			reason:     "unauthorized",
		},
		{
			name:       "wrong bearer token",
			options:    ClientOptions{ClientKey: testClientKey},
			authHeader: "Bearer wrong-key",
			wantStatus: http.StatusUnauthorized,
			reason:     "unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, clientMetrics := newTestMetricsHandler(tt.options)
			resp := postClientMetrics(handler, tt.authHeader, validAppErrorPayload)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status got %d, want %d; body=%s", resp.Code, tt.wantStatus, resp.Body.String())
			}
			if got := testutil.ToFloat64(clientMetrics.RejectedBatches.WithLabelValues(tt.reason)); got != 1 {
				t.Fatalf("rejected batches got %v, want 1", got)
			}
		})
	}
}

func TestClientMetricsAcceptsWhitelistedBatch(t *testing.T) {
	handler, clientMetrics := newTestMetricsHandler(ClientOptions{ClientKey: testClientKey})
	payload := `{
		"events": [
			{"name":"app_error","timestamp":1740000000000,"labels":{"platform":"ios","app_version":"1.0.0","module":"course","level":"error"},"value":2},
			{"name":"app_api_failure","timestamp":1740000000001,"labels":{"platform":"android","api_group":"user","status_code":"500"},"value":1},
			{"name":"app_startup_duration","timestamp":1740000000002,"labels":{"platform":"ios","app_version":"1.0.0"},"value":1.25}
		]
	}`

	resp := postClientMetrics(handler, "Bearer "+testClientKey, payload)
	if resp.Code != http.StatusOK {
		t.Fatalf("status got %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if got := testutil.ToFloat64(clientMetrics.AppErrorsTotal.WithLabelValues("ios", "1.0.0", "course", "error")); got != 2 {
		t.Fatalf("app error counter got %v, want 2", got)
	}
	if got := testutil.ToFloat64(clientMetrics.APIFailuresTotal.WithLabelValues("android", "user", "500")); got != 1 {
		t.Fatalf("api failure counter got %v, want 1", got)
	}
	for _, event := range []string{eventAppError, eventAPIFailure, eventStartupSeconds} {
		if got := testutil.ToFloat64(clientMetrics.IngestedEventsTotal.WithLabelValues(event)); got != 1 {
			t.Fatalf("ingested counter for %s got %v, want 1", event, got)
		}
	}
}

func TestClientMetricsRejectsBatchAtomically(t *testing.T) {
	handler, clientMetrics := newTestMetricsHandler(ClientOptions{ClientKey: testClientKey})
	payload := `{
		"events": [
			{"name":"app_error","timestamp":1740000000000,"labels":{"platform":"ios","app_version":"1.0.0","module":"course","level":"error"},"value":1},
			{"name":"arbitrary_metric","timestamp":1740000000001,"labels":{"user_id":"123456"},"value":1}
		]
	}`

	resp := postClientMetrics(handler, "Bearer "+testClientKey, payload)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status got %d, want 422; body=%s", resp.Code, resp.Body.String())
	}
	if got := testutil.ToFloat64(clientMetrics.AppErrorsTotal.WithLabelValues("ios", "1.0.0", "course", "error")); got != 0 {
		t.Fatalf("batch was partially recorded, app error counter=%v", got)
	}
	if got := testutil.ToFloat64(clientMetrics.RejectedBatches.WithLabelValues("unknown_event")); got != 1 {
		t.Fatalf("unknown event rejection counter got %v, want 1", got)
	}
}

func TestClientMetricsRejectsExtraLabels(t *testing.T) {
	handler, clientMetrics := newTestMetricsHandler(ClientOptions{ClientKey: testClientKey})
	payload := `{"events":[{"name":"app_error","timestamp":1740000000000,"labels":{"platform":"ios","app_version":"1.0.0","module":"course","level":"error","user_id":"123456"},"value":1}]}`

	resp := postClientMetrics(handler, "Bearer "+testClientKey, payload)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status got %d, want 422; body=%s", resp.Code, resp.Body.String())
	}
	if got := testutil.ToFloat64(clientMetrics.RejectedBatches.WithLabelValues("invalid_labels")); got != 1 {
		t.Fatalf("invalid labels rejection counter got %v, want 1", got)
	}
}

func TestClientMetricsCapsAppVersionCardinality(t *testing.T) {
	handler, clientMetrics := newTestMetricsHandler(ClientOptions{
		ClientKey:      testClientKey,
		MaxAppVersions: 1,
	})

	first := postClientMetrics(handler, "Bearer "+testClientKey, validAppErrorPayload)
	if first.Code != http.StatusOK {
		t.Fatalf("first status got %d, want 200; body=%s", first.Code, first.Body.String())
	}
	secondPayload := `{"events":[{"name":"app_startup_duration","timestamp":1740000000001,"labels":{"platform":"ios","app_version":"2.0.0"},"value":1}]}`
	second := postClientMetrics(handler, "Bearer "+testClientKey, secondPayload)
	if second.Code != http.StatusUnprocessableEntity {
		t.Fatalf("second status got %d, want 422; body=%s", second.Code, second.Body.String())
	}
	if got := testutil.ToFloat64(clientMetrics.RejectedBatches.WithLabelValues("cardinality_limit")); got != 1 {
		t.Fatalf("cardinality rejection counter got %v, want 1", got)
	}
}

func TestClientMetricsEnforcesBodyLimit(t *testing.T) {
	handler, clientMetrics := newTestMetricsHandler(ClientOptions{
		ClientKey:    testClientKey,
		MaxBodyBytes: 32,
	})

	resp := postClientMetrics(handler, "Bearer "+testClientKey, validAppErrorPayload)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status got %d, want 413; body=%s", resp.Code, resp.Body.String())
	}
	if got := testutil.ToFloat64(clientMetrics.RejectedBatches.WithLabelValues("payload_too_large")); got != 1 {
		t.Fatalf("payload-too-large rejection counter got %v, want 1", got)
	}
}

const validAppErrorPayload = `{"events":[{"name":"app_error","timestamp":1740000000000,"labels":{"platform":"ios","app_version":"1.0.0","module":"course","level":"error"},"value":1}]}`

func newTestMetricsHandler(options ClientOptions) (*MetricsHandler, *metricsx.ClientMetrics) {
	registry := prometheus.NewRegistry()
	m := metricsx.NewWithRegisterer(registry, "client_test")
	return NewMetricsHandler(nil, m, options), m.Client
}

func postClientMetrics(handler *MetricsHandler, authHeader, payload string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/metrics/client", handler.clientAuthMiddleware(), handler.ClientMetrics)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/client", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}
