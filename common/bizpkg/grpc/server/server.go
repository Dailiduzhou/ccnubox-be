package server

import (
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
	opts, err := cfg.ServerOptions()
	if err != nil {
		panic(err)
	}
	return InitGRPCxKratosServerWithOptions(grpcServer, ecli, l, opts, env, middlewares...)
}

func InitGRPCxKratosServerWithOptions(
	grpcServer GrpcServer,
	ecli *clientv3.Client,
	l logger.Logger,
	opts conf.GRPCServerOptions,
	env *conf.Env,
	middlewares ...middleware.Middleware,
) grpcx.Server {
	if err := opts.Validate(); err != nil {
		panic(err)
	}
	opts.Name = b_grpc.GetNamePrefix(env, opts.Name)
	hs := health.NewServer()
	s := kgrpc.NewServer(
		kgrpc.Address(opts.Addr),
		kgrpc.CustomHealth(),
		kgrpc.Middleware(
			append([]middleware.Middleware{
				recovery.Recovery(),
				tracing.Server(),
				LoggingMiddleware(l),
				HealthMiddleware(hs),
			}, middlewares...)...,
		),
		kgrpc.Timeout(opts.Timeout),
	)
	healthpb.RegisterHealthServer(s, hs)
	hs.SetServingStatus(opts.Name, healthpb.HealthCheckResponse_SERVING)

	grpcServer.Register(s)
	return &grpcx.KratosServer{
		Server:     s,
		Name:       opts.Name,
		Weight:     opts.Weight,
		EtcdTTL:    opts.EtcdTTL,
		EtcdClient: ecli,
		L:          l,
	}
}
