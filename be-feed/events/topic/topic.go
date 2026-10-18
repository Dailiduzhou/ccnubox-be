package topic

// FeedEvent 供生产者和消费者共用，独立包避免 service 与 events 循环依赖。
const FeedEvent = "feed_event"
