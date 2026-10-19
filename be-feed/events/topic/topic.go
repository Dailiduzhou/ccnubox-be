package topic

// FeedEvent 供生产和消费端共享，独立包避免 service 与 events 循环依赖。
const FeedEvent = "feed_event"
