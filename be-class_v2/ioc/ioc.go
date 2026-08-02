package ioc

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	InitLogger,
	InitDB,
	InitRedis,
	InitEtcdClient,
	InitClassListClient,
	InitUserClient,
	InitProxyClient,
	InitHTTPProxyClient,
	InitGRPCServer,
	InitHTTPServer,
	InitMetrics,
	InitMetricsServer,
	InitOTel,
)
