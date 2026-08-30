package ioc

import (
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	InitLogger,
	InitDB,
	InitEtcdClient,
	InitGRPCxKratosServer,
	InitOTel,
	InitUserSvcClient,
	InitContentSvcClient,
	InitProxyClient,
	InitHttpProxyClient,
	InitKafka,
	InitKafkaConsumerClientFactory,
	InitRedis,
	InitMetrics,
	InitMetricsServer,
)
