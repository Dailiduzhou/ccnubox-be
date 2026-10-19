package saramax

import (
	"context"
	"sync"
)

// Consumer 的 Stop 必须取消消费会话、释放资源，并允许重复调用。
type Consumer interface {
	Start() error
	Stop()
}

// StopConsumers 有界地等待消费者停机：消费者可能正卡在一批消息上（单批可达上百条），
// 超时后不再阻塞进程退出，未确认的位点会在下次启动时重投。
// 返回 false 表示等待超时。
func StopConsumers(ctx context.Context, consumers []Consumer) bool {
	done := make(chan struct{})
	var wg sync.WaitGroup
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
	// ConsumeTime 是空闲批次窗口（分钟），<=0 时收到消息即刷新。
	ConsumeTime int `yaml:"consumeTime"`
	// ConsumeNum <=0 时使用 1，避免无效批次大小。
	ConsumeNum int `yaml:"consumeNum"`
	// RetryAttempts 是单次批次在会话内的重试次数上限，<=0 时按默认 4 次。
	// 失败批次最终仍会结束会话并靠 Kafka 重投，长周期故障主要交给消费循环的外层退避。
	RetryAttempts int `yaml:"retryAttempts"`
	// SkipAttempts 是同一条消息永久失败多少次后允许跳过，0 时按 DefaultSkipAttempts、
	// 小于 0 时禁用跳过（永久失败的消息会一直阻塞分区，等人工处理）。
	// 只有 saramax.Permanent 包装的错误参与计数，依赖故障不会被跳过。
	SkipAttempts int `yaml:"skipAttempts"`
}

func normalizeHandlerConfig(cfg *HandlerConfig) *HandlerConfig {
	var result HandlerConfig
	if cfg != nil {
		result = *cfg
	}
	if result.ConsumeNum <= 0 {
		result.ConsumeNum = 1
	}
	if result.ConsumeTime < 0 {
		result.ConsumeTime = 0
	}
	return &result
}
