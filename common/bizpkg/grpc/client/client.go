package client

import (
	"context"
	"fmt"

	ccnuv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/ccnu/v1"
	classv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/classService/v1"
	classlistv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/classlist/v1"
	contentv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/content/v1"
	counterv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/counter/v1"
	elecpricev1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/elecprice/v1"
	feedv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/feed/v1"
	gradev1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/grade/v1"
	libraryv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/library/v1"
	proxyv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/proxy/v1"
	userv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/user/v1"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/conf"
	b_grpc "github.com/asynccnu/ccnubox-be/common/bizpkg/grpc"
	"github.com/go-kratos/kratos/contrib/registry/etcd/v2"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	k_grpc "github.com/go-kratos/kratos/v2/transport/grpc"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

// --- 核心泛型工具函数 ---

func NewGrpcClient(ecli *clientv3.Client, cfg *conf.GrpcConf) *grpc.ClientConn {
	options, err := b_grpc.ClientOptionsFrom(cfg)
	if err != nil {
		panic(fmt.Sprintf("gRPC 客户端配置错误: %v", err))
	}
	return newGrpcClient(ecli, options)
}

func newGrpcClient(ecli *clientv3.Client, options b_grpc.GRPCClientOptions) *grpc.ClientConn {
	r := etcd.New(ecli)
	cc, err := k_grpc.DialInsecure(context.Background(),
		k_grpc.WithEndpoint(fmt.Sprintf("discovery:///%s", options.Name)),
		k_grpc.WithDiscovery(r),
		k_grpc.WithTimeout(options.Timeout),
		k_grpc.WithMiddleware(tracing.Client()),
		k_grpc.WithHealthCheck(false),
	)
	if err != nil {
		panic(fmt.Sprintf("连接服务 %s 失败: %v", options.Name, err))
	}
	return cc
}

func InitClient[T any](ecli *clientv3.Client, cfg *conf.GrpcConf, env *conf.Env, fn func(cc grpc.ClientConnInterface) T) T {
	options, err := b_grpc.ClientOptionsFrom(cfg)
	if err != nil {
		panic(fmt.Sprintf("gRPC 客户端配置错误: %v", err))
	}
	options.Name = b_grpc.GetNamePrefix(env, options.Name)
	return fn(newGrpcClient(ecli, options))
}

// 内部辅助：安全获取配置
func GetConf(cfg *conf.GrpcConfs, key string) *conf.GrpcConf {
	return b_grpc.GetConf(cfg, key)
}

// --- 各服务初始化函数 ---

func InitClassList(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) classlistv1.ClasserClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.CLASSLIST), env, classlistv1.NewClasserClient)
}

func InitUser(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) userv1.UserServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.USER), env, userv1.NewUserServiceClient)
}

func InitGrade(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) gradev1.GradeServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.GRADE), env, gradev1.NewGradeServiceClient)
}

func InitLibrary(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) libraryv1.LibraryServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.LIBRARY), env, libraryv1.NewLibraryServiceClient)
}

func InitContent(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) contentv1.ContentServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.CONTENT), env, contentv1.NewContentServiceClient)
}

func InitElecprice(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) elecpricev1.ElecpriceServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.ELECPRICE), env, elecpricev1.NewElecpriceServiceClient)
}

func InitCCNU(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) ccnuv1.CCNUServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.CCNU), env, ccnuv1.NewCCNUServiceClient)
}

func InitCounter(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) counterv1.CounterServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.COUNTER), env, counterv1.NewCounterServiceClient)
}

func InitFeed(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) feedv1.FeedServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.FEED), env, feedv1.NewFeedServiceClient)
}

func InitProxy(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) proxyv1.ProxyClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.PROXY), env, proxyv1.NewProxyClient)
}

func InitClass(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) classv1.ClassServiceClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.CLASSS), env, classv1.NewClassServiceClient)
}

func InitClassRoom(ecli *clientv3.Client, cfg *conf.GrpcConfs, env *conf.Env) classv1.FreeClassroomSvcClient {
	return InitClient(ecli, GetConf(cfg, b_grpc.CLASSS), env, classv1.NewFreeClassroomSvcClient)
}
