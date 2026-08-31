package service

import (
	"context"
	"strings"
	"time"

	"github.com/asynccnu/ccnubox-be/be-feed/repository/dao"
	"github.com/asynccnu/ccnubox-be/be-feed/repository/model"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
)

const (
	pushDeliveryBatchSize    = 50
	pushDeliveryMaxAttempts  = 10
	pushDeliveryMaxBackoff   = 30 * time.Minute
	pushDeliveryErrorLimit   = 2048
	pushDeliveryStateTimeout = 5 * time.Second
)

type PushDeliveryService interface {
	RecoverSending(ctx context.Context) error
	DispatchDue(ctx context.Context) error
}

type pushDeliveryService struct {
	dao     dao.PushDeliveryDAO
	gate    dao.FeedUserConfigDAO
	push    PushService
	log     logger.Logger
	metrics *metricsx.FeedDeliveryMetrics
}

func (s *pushDeliveryService) RecoverSending(ctx context.Context) error {
	return s.dao.RecoverSending(ctx)
}

func NewPushDeliveryService(deliveryDAO dao.PushDeliveryDAO, gate dao.FeedUserConfigDAO, push PushService, metricSet *metricsx.Metrics, log logger.Logger) PushDeliveryService {
	var deliveryMetrics *metricsx.FeedDeliveryMetrics
	if metricSet != nil {
		deliveryMetrics = metricSet.Feed
	}
	return &pushDeliveryService{
		dao:     deliveryDAO,
		gate:    gate,
		push:    push,
		log:     log,
		metrics: deliveryMetrics,
	}
}

func (s *pushDeliveryService) DispatchDue(ctx context.Context) error {
	// 定时恢复上一个批次遗留的 sending，避免只能依赖服务重启恢复。
	// 当前投递任务串行执行，恢复时不会误抢仍在处理的记录。
	if err := s.dao.RecoverSending(ctx); err != nil {
		return err
	}

	now := time.Now().Unix()
	deliveries, err := s.dao.ListDue(ctx, now, pushDeliveryBatchSize)
	if err != nil {
		return err
	}
	for i := range deliveries {
		claimed, err := s.dao.Claim(ctx, deliveries[i].ID)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		event, err := s.dao.GetFeedEvent(ctx, deliveries[i].FeedEventID)
		if err == nil && strings.EqualFold(event.Type, "library") && s.gate != nil {
			enabled, gateErr := s.gate.IsLibraryEnabled(ctx, event.StudentId)
			if gateErr != nil {
				err = gateErr
			} else if !enabled {
				err = s.markSuppressed(ctx, deliveries[i].ID)
				if err == nil {
					if s.metrics != nil {
						s.metrics.PushDeliveryTotal.WithLabelValues("suppressed_by_allow_list").Inc()
					}
					continue
				}
			}
		}
		if err == nil {
			domainEvent := convFeedEventsFromModelToDomain([]model.FeedEvent{*event})[0]
			err = s.push.PushMSG(ctx, &domainEvent)
		}
		if err == nil {
			err = s.markSent(ctx, deliveries[i].ID)
			if err == nil {
				if s.metrics != nil {
					s.metrics.PushDeliveryTotal.WithLabelValues("sent").Inc()
				}
				continue
			}
		}

		attempts := deliveries[i].Attempts + 1
		backoff := time.Second << min(attempts, 10)
		if backoff > pushDeliveryMaxBackoff {
			backoff = pushDeliveryMaxBackoff
		}
		lastError := boundedPushError(err)
		failed := attempts >= pushDeliveryMaxAttempts
		stateCtx, cancel := pushDeliveryStateContext(ctx)
		markErr := s.dao.MarkRetry(stateCtx, deliveries[i].ID, attempts, time.Now().Add(backoff).Unix(), lastError, failed)
		cancel()
		if markErr != nil {
			return markErr
		}
		if s.metrics != nil {
			result := "retry"
			if failed {
				result = "failed"
			}
			s.metrics.PushDeliveryTotal.WithLabelValues(result).Inc()
		}
		s.log.Warn("push delivery failed",
			logger.Int64("delivery_id", deliveries[i].ID),
			logger.Int("attempt", attempts),
			logger.String("status", map[bool]string{true: "failed", false: "pending"}[failed]),
			logger.Error(err))
	}
	return nil
}

func (s *pushDeliveryService) markSent(ctx context.Context, id int64) error {
	stateCtx, cancel := pushDeliveryStateContext(ctx)
	defer cancel()
	return s.dao.MarkSent(stateCtx, id)
}

func (s *pushDeliveryService) markSuppressed(ctx context.Context, id int64) error {
	stateCtx, cancel := pushDeliveryStateContext(ctx)
	defer cancel()
	return s.dao.MarkSuppressed(stateCtx, id)
}

// pushDeliveryStateContext 确保批次超时后仍有短暂窗口回写已 claim 的记录状态。
func pushDeliveryStateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), pushDeliveryStateTimeout)
}

// boundedPushError 限制错误文本长度，避免被包装的客户端错误包含大量响应内容。
// 数据库字段仅用于诊断，不得存储响应载荷。
func boundedPushError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if len(text) > pushDeliveryErrorLimit {
		return text[:pushDeliveryErrorLimit]
	}
	return text
}
