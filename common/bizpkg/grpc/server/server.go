package server

import (
	"fmt"

	"github.com/asynccnu/ccnubox-be/common/bizpkg/conf"
	b_grpc "github.com/asynccnu/ccnubox-be/common/bizpkg/grpc"
	"github.com/asynccnu/ccnubox-be/common/pkg/grpcx"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type GrpcServer interface {
	Register(server grpc.ServiceRegistrar)
}

func InitGRPCxKratosServer(
	grpcServer GrpcServer,
	ecli *clientv3.Client,
	l logger.Logger,
	cfg *conf.GrpcConf,
	env *conf.Env,
	middlewares ...middleware.Middleware,
) grpcx.Server {
	options, err := b_grpc.ServerOptionsFrom(cfg)
	if err != nil {
		panic(fmt.Sprintf("gRPC 服务端配置错误: %v", err))
	}
	options.Name = b_grpc.GetNamePrefix(env, options.Name)
	return newGRPCxKratosServer(grpcServer, ecli, l, options, middlewares...)
}

func newGRPCxKratosServer(
	grpcServer GrpcServer,
	ecli *clientv3.Client,
	l logger.Logger,
	options b_grpc.GRPCServerOptions,
	middlewares ...middleware.Middleware,
) grpcx.Server {
	hs := health.NewServer()
	s := kgrpc.NewServer(
		kgrpc.Address(options.Addr),
		kgrpc.CustomHealth(),
		kgrpc.Middleware(
			append([]middleware.Middleware{
				recovery.Recovery(),
				tracing.Server(),
				LoggingMiddleware(l),
				HealthMiddleware(hs),
			}, middlewares...)...,
		),
		kgrpc.Timeout(options.Timeout),
	)
	healthpb.RegisterHealthServer(s, hs)
	hs.SetServingStatus(options.Name, healthpb.HealthCheckResponse_SERVING)

	grpcServer.Register(s)
	return &grpcx.KratosServer{
		Server:     s,
		Name:       options.Name,
		Weight:     options.Weight,
		EtcdTTL:    options.EtcdTTL,
		EtcdClient: ecli,
		L:          l,
	}
}
