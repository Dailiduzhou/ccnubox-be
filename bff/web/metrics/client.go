package metrics

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/asynccnu/ccnubox-be/bff/web"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"github.com/gin-gonic/gin"
)

const (
	defaultMaxBatchSize   = 100
	defaultMaxBodyBytes   = int64(64 << 10)
	defaultMaxAppVersions = 32

	eventAppError       = "app_error"
	eventAPIFailure     = "app_api_failure"
	eventStartupSeconds = "app_startup_duration"
)

var (
	appVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$`)
	httpStatusPattern = regexp.MustCompile(`^[1-5][0-9]{2}$`)

	allowedPlatforms = stringSet("ios", "android")
	allowedLevels    = stringSet("error", "fatal")
	allowedModules   = stringSet("course", "auth", "jpush", "electric")
	allowedAPIGroups = stringSet(
		"course", "user", "feedback", "auth", "electric", "feed",
		"jpush", "library", "grade", "classroom", "content",
	)
	allowedNamedStatuses = stringSet("timeout", "network_timeout", "network_error", "cancelled", "unknown")
)

type ClientOptions struct {
	ClientKey      string
	MaxBatchSize   int
	MaxBodyBytes   int64
	MaxAppVersions int
}

type ClientMetricsReq struct {
	Events []ClientMetricEvent `json:"events"`
}

type ClientMetricEvent struct {
	Name      string            `json:"name"`
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	Value     float64           `json:"value"`
}

type clientCollector struct {
	metrics       *metricsx.ClientMetrics
	clientKeyHash [sha256.Size]byte
	keyConfigured bool
	maxBatchSize  int
	maxBodyBytes  int64
	versionGuard  *versionCardinalityGuard
}

type versionCardinalityGuard struct {
	mu       sync.Mutex
	versions map[string]struct{}
	max      int
}

type validationError struct {
	reason string
	msg    string
}

func (e *validationError) Error() string { return e.msg }

func newClientCollector(m *metricsx.ClientMetrics, options ClientOptions) *clientCollector {
	if options.MaxBatchSize <= 0 {
		options.MaxBatchSize = defaultMaxBatchSize
	}
	if options.MaxBodyBytes <= 0 {
		options.MaxBodyBytes = defaultMaxBodyBytes
	}
	if options.MaxAppVersions <= 0 {
		options.MaxAppVersions = defaultMaxAppVersions
	}

	collector := &clientCollector{
		metrics:      m,
		maxBatchSize: options.MaxBatchSize,
		maxBodyBytes: options.MaxBodyBytes,
		versionGuard: &versionCardinalityGuard{
			versions: make(map[string]struct{}, options.MaxAppVersions),
			max:      options.MaxAppVersions,
		},
	}
	if options.ClientKey != "" {
		collector.clientKeyHash = sha256.Sum256([]byte(options.ClientKey))
		collector.keyConfigured = true
	}
	return collector
}

func (h *MetricsHandler) clientAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !h.client.keyConfigured {
			h.client.reject("collector_disabled")
			ctx.AbortWithStatusJSON(http.StatusServiceUnavailable, web.Response{
				Code: http.StatusServiceUnavailable,
				Msg:  "client metrics collector is disabled",
			})
			return
		}

		token, ok := parseBearerToken(ctx.GetHeader("Authorization"))
		providedHash := sha256.Sum256([]byte(token))
		if !ok || subtle.ConstantTimeCompare(providedHash[:], h.client.clientKeyHash[:]) != 1 {
			h.client.reject("unauthorized")
			ctx.Header("WWW-Authenticate", `Bearer realm="client-metrics"`)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, web.Response{
				Code: http.StatusUnauthorized,
				Msg:  "invalid client metrics token",
			})
			return
		}

		ctx.Next()
	}
}

// ClientMetrics accepts a validated batch and records it in the in-process
// Prometheus registry. The whole batch is validated before any metric changes.
// Event timestamps describe when the client observed an event; Prometheus still
// records the sample at ingestion time, which is the expected collector model.
// @Summary 批量接收移动端 Prometheus 指标
// @Description 使用独立 App Client Key 的 Bearer Token 鉴权；事件名、Label 名称和值均执行白名单校验
// @Tags metrics
// @Accept json
// @Produce json
// @Param data body ClientMetricsReq true "客户端指标批次"
// @Success 200 {object} web.Response{data=map[string]int} "接收成功"
// @Failure 400 {object} web.Response "JSON 非法"
// @Failure 401 {object} web.Response "Client Key 无效"
// @Failure 413 {object} web.Response "请求体过大"
// @Failure 422 {object} web.Response "事件或 Label 不符合白名单"
// @Failure 503 {object} web.Response "Collector 未配置"
// @Router /metrics/client [post]
func (h *MetricsHandler) ClientMetrics(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, h.client.maxBodyBytes)

	var req ClientMetricsReq
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		h.writeDecodeError(ctx, err)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		h.writeDecodeError(ctx, err)
		return
	}

	if err := h.client.validate(req); err != nil {
		h.client.reject(err.reason)
		ctx.AbortWithStatusJSON(http.StatusUnprocessableEntity, web.Response{
			Code: http.StatusUnprocessableEntity,
			Msg:  err.Error(),
		})
		return
	}

	h.client.record(req.Events)
	ctx.AbortWithStatusJSON(http.StatusOK, web.Response{
		Code: http.StatusOK,
		Msg:  "client metrics accepted",
		Data: map[string]int{"accepted": len(req.Events)},
	})
}

func (h *MetricsHandler) writeDecodeError(ctx *gin.Context, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		h.client.reject("payload_too_large")
		ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, web.Response{
			Code: http.StatusRequestEntityTooLarge,
			Msg:  "metrics payload is too large",
		})
		return
	}
	h.client.reject("invalid_json")
	ctx.AbortWithStatusJSON(http.StatusBadRequest, web.Response{
		Code: http.StatusBadRequest,
		Msg:  "invalid metrics payload",
	})
}

func (c *clientCollector) validate(req ClientMetricsReq) *validationError {
	if len(req.Events) == 0 {
		return invalid("empty_batch", "events must contain at least one item")
	}
	if len(req.Events) > c.maxBatchSize {
		return invalid("batch_too_large", fmt.Sprintf("events must contain at most %d items", c.maxBatchSize))
	}

	for i, event := range req.Events {
		if err := validateEvent(event); err != nil {
			return invalid(err.reason, fmt.Sprintf("events[%d]: %s", i, err.msg))
		}
	}
	if !c.versionGuard.reserve(req.Events) {
		return invalid("cardinality_limit", "app_version cardinality limit exceeded")
	}
	return nil
}

func validateEvent(event ClientMetricEvent) *validationError {
	if event.Timestamp <= 0 {
		return invalid("invalid_timestamp", "timestamp must be a positive Unix millisecond value")
	}
	if math.IsNaN(event.Value) || math.IsInf(event.Value, 0) || event.Value <= 0 {
		return invalid("invalid_value", "value must be a finite positive number")
	}

	switch event.Name {
	case eventAppError:
		if event.Value != math.Trunc(event.Value) || event.Value > 1_000_000 {
			return invalid("invalid_value", "counter value must be an integer between 1 and 1000000")
		}
		if err := validateLabels(event.Labels, "platform", "app_version", "module", "level"); err != nil {
			return err
		}
		if !allowedPlatforms[event.Labels["platform"]] || !appVersionPattern.MatchString(event.Labels["app_version"]) ||
			!allowedModules[event.Labels["module"]] || !allowedLevels[event.Labels["level"]] {
			return invalid("invalid_label_value", "one or more label values are not allowed")
		}
	case eventAPIFailure:
		if event.Value != math.Trunc(event.Value) || event.Value > 1_000_000 {
			return invalid("invalid_value", "counter value must be an integer between 1 and 1000000")
		}
		if err := validateLabels(event.Labels, "platform", "api_group", "status_code"); err != nil {
			return err
		}
		status := event.Labels["status_code"]
		if !allowedPlatforms[event.Labels["platform"]] || !allowedAPIGroups[event.Labels["api_group"]] ||
			(!httpStatusPattern.MatchString(status) && !allowedNamedStatuses[status]) {
			return invalid("invalid_label_value", "one or more label values are not allowed")
		}
	case eventStartupSeconds:
		if event.Value > 300 {
			return invalid("invalid_value", "startup duration must not exceed 300 seconds")
		}
		if err := validateLabels(event.Labels, "platform", "app_version"); err != nil {
			return err
		}
		if !allowedPlatforms[event.Labels["platform"]] || !appVersionPattern.MatchString(event.Labels["app_version"]) {
			return invalid("invalid_label_value", "one or more label values are not allowed")
		}
	default:
		return invalid("unknown_event", "event name is not allowed")
	}
	return nil
}

func validateLabels(labels map[string]string, required ...string) *validationError {
	if len(labels) != len(required) {
		return invalid("invalid_labels", "labels must exactly match the event schema")
	}
	for _, key := range required {
		if labels[key] == "" {
			return invalid("invalid_labels", "labels must exactly match the event schema")
		}
	}
	return nil
}

func (c *clientCollector) record(events []ClientMetricEvent) {
	for _, event := range events {
		switch event.Name {
		case eventAppError:
			c.metrics.AppErrorsTotal.WithLabelValues(
				event.Labels["platform"], event.Labels["app_version"], event.Labels["module"], event.Labels["level"],
			).Add(event.Value)
		case eventAPIFailure:
			c.metrics.APIFailuresTotal.WithLabelValues(
				event.Labels["platform"], event.Labels["api_group"], event.Labels["status_code"],
			).Add(event.Value)
		case eventStartupSeconds:
			c.metrics.StartupDuration.WithLabelValues(
				event.Labels["platform"], event.Labels["app_version"],
			).Observe(event.Value)
		}
		c.metrics.IngestedEventsTotal.WithLabelValues(event.Name).Inc()
	}
}

func (c *clientCollector) reject(reason string) {
	c.metrics.RejectedBatches.WithLabelValues(reason).Inc()
}

func (g *versionCardinalityGuard) reserve(events []ClientMetricEvent) bool {
	requested := make(map[string]struct{})
	for _, event := range events {
		if version := event.Labels["app_version"]; version != "" {
			requested[version] = struct{}{}
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	newCount := 0
	for version := range requested {
		if _, exists := g.versions[version]; !exists {
			newCount++
		}
	}
	if len(g.versions)+newCount > g.max {
		return false
	}
	for version := range requested {
		g.versions[version] = struct{}{}
	}
	return true
}

func parseBearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("metrics payload must contain exactly one JSON object")
}

func invalid(reason, msg string) *validationError {
	return &validationError{reason: reason, msg: msg}
}

func stringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
