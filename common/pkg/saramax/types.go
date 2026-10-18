package saramax

import (
	"context"
	"sync"
	"time"
)

// Consumer 的 Stop 必须先取消消费会话，再等待退出，并支持重复调用。
type Consumer interface {
	Start() error
	Stop()
}

// StopConsumers 有界地等待消费者停机：消费者可能正卡在一批消息上（单批可达上百条），
// 超时后不再阻塞进程退出，未确认的位点会在下次启动时重投。
// 返回 false 表示等待超时。
func StopConsumers(ctx context.Context, consumers []Consumer) bool {
	if len(consumers) == 0 {
		return true
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	// 先并发通知所有消费者，不能让一个阻塞的 Stop 推迟其他消费者的取消。
	for _, c := range consumers {
		wg.Add(1)
		go func(c Consumer) {
			defer wg.Done()
			c.Stop()
		}(c)
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

type HandlerConfig struct {
	// ConsumeTime 单位为分钟，<=0 保留立即刷新缓冲消息的兼容语义。
	ConsumeTime int `yaml:"consumeTime"`
	// ConsumeNum <=0 时按单条消费。
	ConsumeNum int `yaml:"consumeNum"`
	// RetryAttempts 是单次批次在会话内的重试次数上限，<=0 时按默认 4 次。
	// 失败批次最终仍会结束会话并靠 Kafka 重投，长周期故障主要交给消费循环的外层退避。
	RetryAttempts int `yaml:"retryAttempts"`
	// SkipAttempts 是同一条消息永久失败多少次后允许跳过，0 时按 DefaultSkipAttempts、
	// 小于 0 时禁用跳过（永久失败的消息会一直阻塞分区，等人工处理）。
	// 只有 saramax.Permanent 包装的错误参与计数，依赖故障不会被跳过。
	SkipAttempts int `yaml:"skipAttempts"`
}

// normalizeHandlerConfig 创建独立配置快照，不修改调用方共享配置。
func normalizeHandlerConfig(cfg *HandlerConfig) *HandlerConfig {
	var normalized HandlerConfig
	if cfg != nil {
		normalized = *cfg
	}
	if normalized.ConsumeNum <= 0 {
		normalized.ConsumeNum = 1
	}
	if normalized.ConsumeTime < 0 {
		normalized.ConsumeTime = 0
	}
	if int64(normalized.ConsumeTime) > (1<<63-1)/int64(time.Minute) {
		panic("Kafka consumeTime exceeds duration limit")
	}
	return &normalized
}
