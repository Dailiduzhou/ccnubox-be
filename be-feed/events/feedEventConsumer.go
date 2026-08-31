package events

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/asynccnu/ccnubox-be/be-feed/domain"
	"github.com/asynccnu/ccnubox-be/be-feed/events/consumer"
	"github.com/asynccnu/ccnubox-be/be-feed/events/topic"
	"github.com/asynccnu/ccnubox-be/be-feed/service"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
)

// FeedEventConsumerHandler 是处理 Feed 事件消费的结构体
type FeedEventConsumerHandler struct {
	cg          consumer.Consumer        //消费者
	l           logger.Logger            // 日志记录器
	stopChan    chan struct{}            //用于停止的管道,没用上
	feedService service.FeedEventService // 事件数据的存储库
	m           *metricsx.Metrics
	ctx         context.Context
	cancel      context.CancelFunc
	stopOnce    sync.Once
	wg          sync.WaitGroup
}

// NewFeedEventConsumerHandler 是 FeedEventConsumerHandler 的构造函数
// 接收 Kafka 客户端、日志记录器和事件存储库作为参数，并返回一个 FeedEventConsumerHandler 实例
func NewFeedEventConsumerHandler(
	kafkaClient sarama.Client,
	l logger.Logger,
	feedService service.FeedEventService,
	m *metricsx.Metrics,
) *FeedEventConsumerHandler {
	cg := consumer.NewSaramaConsumer(kafkaClient, topic.FeedEvent)
	ctx, cancel := context.WithCancel(context.Background())
	return &FeedEventConsumerHandler{
		cg:          cg,
		l:           l,
		feedService: feedService,
		stopChan:    make(chan struct{}),
		m:           m,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动事件消费的流程
func (f *FeedEventConsumerHandler) Start() error {

	// 启动一个 Goroutine 异步消费消息
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		for {
			f.l.Info("开始消费")
			// 开始消费主题为 "feed_event" 的消息，并使用自定义的处理函数
			er := f.cg.Consume(f.ctx, []string{topic.FeedEvent}, &feedEventKafkaHandler{consumer: f})
			if er != nil {
				if f.ctx.Err() != nil {
					return
				}
				// 如果消费循环中出现错误，记录错误日志
				f.l.Error("退出了消费循环异常", logger.Error(er))
				time.Sleep(time.Second)
				continue
			}
			f.l.Info("消费者停止消费")
		}

	}()
	return nil
}

func (f *FeedEventConsumerHandler) Stop() {
	f.stopOnce.Do(func() {
		if f.cancel != nil {
			f.cancel()
		}
		if f.cg != nil {
			if err := f.cg.Close(); err != nil {
				f.l.Error("close feed consumer failed", logger.Error(err))
			}
		}
	})
	f.wg.Wait()
}

// Consume 是实际处理 Kafka 消息的函数
// 接收 Kafka 消息和事件数组作为参数,并存储到到临时变量里面去
func (f *FeedEventConsumerHandler) Consume(events []domain.FeedEvent) error {
	return f.consume(context.Background(), events)
}

func (f *FeedEventConsumerHandler) consume(ctx context.Context, events []domain.FeedEvent) error {
	err := f.feedService.InsertEventList(ctx, events)
	if err != nil {
		f.recordFailure("db_error", len(events))
		return err
	}
	f.recordConsumed("OK", len(events))
	return nil
}

const maxFeedConsumeAttempts = 4

func (f *FeedEventConsumerHandler) recordFailure(errorType string, count int) {
	if count <= 0 || f.m == nil || f.m.MQMetrics == nil || f.m.MQMetrics.FailedTotal == nil {
		return
	}
	f.m.MQMetrics.FailedTotal.WithLabelValues(topic.FeedEvent, errorType).Add(float64(count))
}

func (f *FeedEventConsumerHandler) recordConsumed(status string, count int) {
	if count <= 0 || f.m == nil || f.m.MQMetrics == nil || f.m.MQMetrics.ConsumedTotal == nil {
		return
	}
	f.m.MQMetrics.ConsumedTotal.WithLabelValues(topic.FeedEvent, status).Add(float64(count))
}

// feedEventKafkaHandler 按分区顺序处理消息。格式或字段非法的消息无法通过重试恢复，
// 因此记录足够的定位信息和指标后确认；数据库错误则有限退避重试，耗尽后不
// 确认 offset，让 Kafka 在下一次 consumer session 中重新投递。
type feedEventKafkaHandler struct {
	consumer *FeedEventConsumerHandler
}

func (h *feedEventKafkaHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *feedEventKafkaHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *feedEventKafkaHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return nil
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			ack, err := h.consumeMessage(session.Context(), message)
			if ack {
				session.MarkMessage(message, "")
			}
			if err != nil {
				return err
			}
		}
	}
}

func (h *feedEventKafkaHandler) consumeMessage(ctx context.Context, message *sarama.ConsumerMessage) (bool, error) {
	logh := h.consumer.l.WithContext(ctx)
	fields := []logger.Field{
		logger.String("topic", message.Topic),
		logger.Int32("partition", message.Partition),
		logger.Int64("offset", message.Offset),
		logger.Int("payload_size", len(message.Value)),
	}

	var event domain.FeedEvent
	if err := json.Unmarshal(message.Value, &event); err != nil {
		h.consumer.recordFailure("decode_error", 1)
		h.consumer.recordConsumed("Discarded", 1)
		logh.Error("feed event message discarded: invalid payload", append(fields, logger.Error(err))...)
		return true, nil
	}
	if len(event.Url) > domain.MaxFeedEventURLBytes {
		h.consumer.recordFailure("invalid_event", 1)
		h.consumer.recordConsumed("Discarded", 1)
		logh.Error("feed event message discarded: url exceeds storage limit",
			append(fields, logger.Int("url_size", len(event.Url)))...)
		return true, nil
	}

	var consumeErr error
	for attempt := 1; attempt <= maxFeedConsumeAttempts; attempt++ {
		consumeErr = h.consumer.feedService.InsertEventList(ctx, []domain.FeedEvent{event})
		if consumeErr == nil {
			h.consumer.recordConsumed("OK", 1)
			if attempt > 1 {
				logh.Info("feed event message stored after retry", append(fields, logger.Int("attempts", attempt))...)
			}
			return true, nil
		}
		if ctx.Err() != nil {
			return false, nil
		}
		if attempt == maxFeedConsumeAttempts {
			break
		}

		delay := time.Duration(1<<(attempt-1)) * 100 * time.Millisecond
		h.consumer.recordFailure("db_retry", 1)
		logh.Warn("feed event storage failed; retrying",
			append(fields,
				logger.Int("attempt", attempt),
				logger.Int64("retry_delay_ms", delay.Milliseconds()),
				logger.Error(consumeErr),
			)...)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, nil
		case <-timer.C:
		}
	}

	h.consumer.recordFailure("db_error", 1)
	logh.Error("feed event storage retries exhausted; offset left uncommitted",
		append(fields,
			logger.Int("attempts", maxFeedConsumeAttempts),
			logger.Error(consumeErr),
		)...)
	return false, consumeErr
}
