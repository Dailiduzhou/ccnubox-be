package consumer

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"github.com/asynccnu/ccnubox-be/common/pkg/otelx/otelsarama"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// DelaySendHandler 消费延迟 topic消息并转发到真实 topic
type DelaySendHandler struct {
	delayTopic    string
	topic         string
	kp            sarama.SyncProducer
	delayTime     time.Duration
	log           logger.Logger
	setOnce       sync.Once
	downOnce      sync.Once
	producedTotal *prometheus.CounterVec
	consumedTotal *prometheus.CounterVec
	mqFailedTotal *prometheus.CounterVec
}

func NewDelaySendHandler(delayTopic, topic string, client sarama.Client, delayTime time.Duration, l logger.Logger, m *metricsx.Metrics) (*DelaySendHandler, error) {
	kp, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		return nil, err
	}
	return &DelaySendHandler{
		delayTopic:    delayTopic,
		topic:         topic,
		kp:            kp,
		delayTime:     delayTime,
		log:           l,
		producedTotal: m.MQMetrics.ProducedTotal,
		consumedTotal: m.MQMetrics.ConsumedTotal,
		mqFailedTotal: m.MQMetrics.FailedTotal,
	}, nil
}

func (c *DelaySendHandler) Setup(sarama.ConsumerGroupSession) error {
	c.setOnce.Do(func() {
		c.log.Infof("delay send handler setup")
	})
	return nil
}

func (c *DelaySendHandler) Cleanup(sarama.ConsumerGroupSession) error {
	c.downOnce.Do(func() {
		c.log.Infof("delay send handler cleanup")
	})
	return nil
}

func (c *DelaySendHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		if !c.processMessage(session, message) {
			// session 已结束（rebalance 或关闭），当前消息未提交，下个 generation 会重新投递。
			// 这里必须返回 nil：sarama 只把 ConsumeClaim 的错误交给 handleError，
			// 并不会结束 session，返回错误只会让本分区在剩余 session 里静默停摆。
			return nil
		}
		session.MarkMessage(message, "")
	}
	return nil
}

// processMessage 等待消息到期并转发，临时失败则做有限次原地退避重试。
// 返回 false 表示 session 已结束，当前消息不得提交 offset。
func (c *DelaySendHandler) processMessage(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage) bool {
	// 未到期就原地等到期。分区内消息按投递时间有序，阻塞本分区正是延迟队列想要的语义。
	for {
		dur := time.Since(message.Timestamp)
		if dur >= c.delayTime {
			break
		}
		if !sleepWithContext(session.Context(), c.delayTime-dur) {
			return false
		}
	}

	// 严重滞后的消息直接丢弃，不再转发。
	if c.delayTime > 0 && time.Since(message.Timestamp) >= 20*c.delayTime {
		return true
	}

	backoff := initialRetryBackoff
	for attempt := 1; ; attempt++ {
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), otelsarama.NewConsumerMessageCarrier(message))

		tracer := otel.Tracer("delay-queue-consume")
		ctx, span := tracer.Start(ctx, "delay-queue-consume",
			trace.WithSpanKind(trace.SpanKindConsumer),
		)

		tlog := c.log.WithContext(ctx)
		tlog.Debugf("Message claimed: key:%s, value:%s, time_sub:%v",
			string(message.Key), string(message.Value), time.Since(message.Timestamp))

		err := c.forwardMessage(ctx, message)
		if err == nil {
			// 消费计数
			if c.producedTotal != nil {
				c.producedTotal.WithLabelValues(c.topic, "OK").Inc()
			}
			if c.consumedTotal != nil {
				c.consumedTotal.WithLabelValues(c.delayTopic, "OK").Inc()
			}
			span.End()
			return true
		}

		tlog.Errorf("Error forwarding message: %s: %v", string(message.Value), err)
		span.RecordError(err)
		if c.mqFailedTotal != nil {
			c.mqFailedTotal.WithLabelValues(c.topic, classifyError(err)).Inc()
		}

		// 保持原有行为：永久错误和未知错误记录后确认，避免毒消息永久阻塞分区。
		if !isRetryableKafkaError(err) {
			tlog.Errorf("Non-retryable forwarding error, acknowledging message: topic=%s partition=%d offset=%d err=%v",
				message.Topic, message.Partition, message.Offset, err)
			span.End()
			return true
		}
		// 临时错误只做有限次原地重试；耗尽后仍按原有行为确认消息。
		if attempt >= maxImmediateRetryAttempts {
			tlog.Errorf("Forwarding retries exhausted, acknowledging message: topic=%s partition=%d offset=%d attempts=%d err=%v",
				message.Topic, message.Partition, message.Offset, attempt, err)
			span.End()
			return true
		}

		tlog.Warnf("Retrying message forwarding in %v: topic=%s partition=%d offset=%d attempt=%d",
			backoff, message.Topic, message.Partition, message.Offset, attempt)
		span.End()

		if !sleepWithContext(session.Context(), backoff) {
			return false
		}
		backoff = nextBackoff(backoff)
	}
}

func (c *DelaySendHandler) forwardMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	otel.GetTextMapPropagator().Inject(ctx, otelsarama.NewConsumerMessageCarrier(msg))

	tlog := c.log.WithContext(ctx)

	_, _, err := c.kp.SendMessage(&sarama.ProducerMessage{
		Topic: c.topic,
		Key:   sarama.ByteEncoder(msg.Key),
		Value: sarama.ByteEncoder(msg.Value),
	})
	if err == nil {
		tlog.Debugf("Forwarded message: key=%s,val=%s,timestamp=%v, current-time=%v", string(msg.Key), string(msg.Value), msg.Timestamp, time.Now())
	}
	return err
}

// FuncConsumeHandler 消费真实 topic 消息并交付给应用
type FuncConsumeHandler struct {
	f             func(ctx context.Context, key []byte, value []byte) (ack bool, err error)
	log           logger.Logger
	consumedTotal *prometheus.CounterVec
	mqFailedTotal *prometheus.CounterVec
}

func NewFuncConsumeHandler(f func(ctx context.Context, key []byte, value []byte) (ack bool, err error), l logger.Logger, m *metricsx.Metrics) FuncConsumeHandler {
	return FuncConsumeHandler{
		f:             f,
		log:           l,
		consumedTotal: m.MQMetrics.ConsumedTotal,
		mqFailedTotal: m.MQMetrics.FailedTotal,
	}
}

func (fc FuncConsumeHandler) Setup(sarama.ConsumerGroupSession) error {
	fc.log.Info("Setting up func consume handler")
	return nil
}

func (fc FuncConsumeHandler) Cleanup(sarama.ConsumerGroupSession) error {
	fc.log.Info("Cleaning up func consume handler")
	return nil
}

func (fc FuncConsumeHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		if !fc.handleWithRetry(session, message) {
			return nil
		}
		// 业务失败也可以确认：例如下一条延迟重试已发布，或错误已判定为终态。
		session.MarkMessage(message, "")
	}
	return nil
}

// handleWithRetry 对未确认消息做有限次原地重试。
// 返回 false 表示 session 已结束，当前消息不得提交 offset。
func (fc FuncConsumeHandler) handleWithRetry(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage) bool {
	backoff := initialRetryBackoff
	for attempt := 1; ; attempt++ {
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), otelsarama.NewConsumerMessageCarrier(message))

		tracer := otel.Tracer("real-topic")
		ctx, span := tracer.Start(ctx, "real_topic_consumer",
			trace.WithSpanKind(trace.SpanKindConsumer),
		)

		tlog := fc.log.WithContext(ctx)

		tlog.Debugf("Message claimed: key:%s, value:%s", string(message.Key), string(message.Value))
		ack, err := fc.f(ctx, message.Key, message.Value)
		if !ack && err == nil {
			err = errors.New("message requested retry without an error")
		}
		if err != nil {
			tlog.Errorf("Error handling message: %v", err)
			span.RecordError(err)
			if fc.consumedTotal != nil {
				fc.consumedTotal.WithLabelValues(message.Topic, "Error").Inc()
			}
			if fc.mqFailedTotal != nil {
				fc.mqFailedTotal.WithLabelValues(message.Topic, classifyError(err)).Inc()
			}
		} else if fc.consumedTotal != nil {
			fc.consumedTotal.WithLabelValues(message.Topic, "OK").Inc()
		}
		span.End()

		if ack {
			return true
		}
		// 保持原有行为：有限重试耗尽后确认消息，避免永久阻塞当前分区。
		if attempt >= maxImmediateRetryAttempts {
			tlog.Errorf("Message retries exhausted, acknowledging message: topic=%s partition=%d offset=%d attempts=%d err=%v",
				message.Topic, message.Partition, message.Offset, attempt, err)
			return true
		}

		tlog.Warnf("Message not acked, retrying in %v: topic=%s partition=%d offset=%d attempt=%d",
			backoff, message.Topic, message.Partition, message.Offset, attempt)
		if !sleepWithContext(session.Context(), backoff) {
			return false
		}
		backoff = nextBackoff(backoff)
	}
}

const maxImmediateRetryAttempts = 4

var (
	initialRetryBackoff = time.Second
	maxRetryBackoff     = 30 * time.Second
)

// sleepWithContext 退避等待。返回 false 表示 ctx 已结束（rebalance 或关闭），
// 调用方必须立即退出并且不得提交当前 offset。
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > maxRetryBackoff {
		return maxRetryBackoff
	}
	return next
}

// isRetryableKafkaError 只对白名单中的明确临时错误返回 true。
// 永久错误和未知错误默认不重试，以保持原有的确认行为。
func isRetryableKafkaError(err error) bool {
	var producerErr *sarama.ProducerError
	if errors.As(err, &producerErr) {
		err = producerErr.Err
	}

	if errors.Is(err, sarama.ErrOutOfBrokers) ||
		errors.Is(err, sarama.ErrLeaderNotAvailable) ||
		errors.Is(err, sarama.ErrNotLeaderForPartition) ||
		errors.Is(err, sarama.ErrRequestTimedOut) ||
		errors.Is(err, sarama.ErrNotEnoughReplicas) ||
		errors.Is(err, sarama.ErrNotEnoughReplicasAfterAppend) ||
		errors.Is(err, sarama.ErrUnknownTopicOrPartition) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isRetryableConsumerGroupError(err error) bool {
	return isRetryableKafkaError(err) ||
		errors.Is(err, sarama.ErrNotCoordinatorForConsumer) ||
		errors.Is(err, sarama.ErrConsumerCoordinatorNotAvailable) ||
		errors.Is(err, sarama.ErrOffsetsLoadInProgress) ||
		errors.Is(err, sarama.ErrRebalanceInProgress)
}

type Consumer struct {
	cctx       context.Context
	cancelFunc context.CancelFunc
	newClient  ClientFactory
	newGroup   consumerGroupFactory
	log        logger.Logger
}

type ClientFactory func() sarama.Client

type consumerGroupFactory func(groupID string, client sarama.Client) (sarama.ConsumerGroup, error)

func NewConsumer(newClient ClientFactory, l logger.Logger) *Consumer {
	cctx, cancel := context.WithCancel(context.Background())
	return &Consumer{
		cctx:       cctx,
		cancelFunc: cancel,
		newClient:  newClient,
		newGroup:   sarama.NewConsumerGroupFromClient,
		log:        l,
	}
}

func (c *Consumer) Consume(topics []string, groupID string, handler sarama.ConsumerGroupHandler) error {
	// ConsumerGroup 不共用 Client
	if c.newClient == nil {
		return ErrNilClient
	}
	client := c.newClient()
	if client == nil {
		return ErrNilClient
	}
	cg, err := c.newGroup(groupID, client)
	if err != nil {
		if closeErr := client.Close(); closeErr != nil {
			c.log.Errorf("Error closing Kafka client after consumer group creation failed: %v", closeErr)
		}
		return err
	}
	defer func() {
		// Client 生命周期比 ConsumerGroup 长
		if err := cg.Close(); err != nil {
			c.log.Errorf("Error closing consumer group %s: %v", groupID, err)
		}
		if err := client.Close(); err != nil {
			c.log.Errorf("Error closing Kafka client for consumer group %s: %v", groupID, err)
		}
	}()

	backoff := initialRetryBackoff
	consecutiveFailures := 0
	for {
		err := cg.Consume(c.cctx, topics, handler)
		if c.cctx.Err() != nil {
			return c.cctx.Err()
		}
		if err != nil {
			// 永久错误和未知错误保持原有行为，直接返回给调用方。
			if !isRetryableConsumerGroupError(err) {
				return err
			}
			consecutiveFailures++
			if consecutiveFailures >= maxImmediateRetryAttempts {
				return err
			}
			c.log.Errorf("consumer group %s session ended with temporary error, retrying in %v: attempt=%d err=%v",
				groupID, backoff, consecutiveFailures, err)
			if !sleepWithContext(c.cctx, backoff) {
				return c.cctx.Err()
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = initialRetryBackoff
		consecutiveFailures = 0
	}
}

func (c *Consumer) Close() {
	if c.cancelFunc != nil {
		c.log.Infof("Consumer is shutting down, cancelling context")
		c.cancelFunc()
	}
}

var (
	ErrInvalidGroupID = errors.New("the groupID is not allowed")
	ErrNilClient      = errors.New("kafka client factory returned nil")
)

// classifyError 将 Kafka/Sarama 错误分类，用于 mq_failed_total 标签
func classifyError(err error) string {
	var producerErr *sarama.ProducerError
	if errors.As(err, &producerErr) {
		err = producerErr.Err
	}

	if errors.Is(err, sarama.ErrLeaderNotAvailable) {
		return "leader_not_available"
	}
	if errors.Is(err, sarama.ErrNotEnoughReplicas) || errors.Is(err, sarama.ErrNotEnoughReplicasAfterAppend) {
		return "not_enough_replicas"
	}
	if errors.Is(err, sarama.ErrMessageTooLarge) || errors.Is(err, sarama.ErrMessageSizeTooLarge) {
		return "message_too_large"
	}
	if errors.Is(err, sarama.ErrInvalidTopic) {
		return "invalid_topic"
	}
	return "consume_error"
}
