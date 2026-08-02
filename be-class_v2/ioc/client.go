package ioc

import (
	"github.com/asynccnu/ccnubox-be/be-class_v2/conf"
	classlistv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/classlist/v1"
	proxyv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/proxy/v1"
	userv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/user/v1"
	grpcclient "github.com/asynccnu/ccnubox-be/common/bizpkg/grpc/client"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/proxy"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func InitClassListClient(etcd *clientv3.Client, cfg *conf.InfraConf) classlistv1.ClasserClient {
	return grpcclient.InitClassList(etcd, cfg.Grpc, cfg.Env)
}

func InitUserClient(etcd *clientv3.Client, cfg *conf.InfraConf) userv1.UserServiceClient {
	return grpcclient.InitUser(etcd, cfg.Grpc, cfg.Env)
}

func InitProxyClient(etcd *clientv3.Client, cfg *conf.InfraConf) proxyv1.ProxyClient {
	if cfg.Proxy.IsDirect() {
		return nil
	}
	return grpcclient.InitProxy(etcd, cfg.Grpc, cfg.Env)
}

func InitHTTPProxyClient(client proxyv1.ProxyClient, cfg *conf.InfraConf, l logger.Logger) proxy.Client {
	if cfg.Proxy.IsDirect() {
		return proxy.NewDirectHttpProxy(l)
	}
	return proxy.NewHttpProxy(client, l)
}
