package ioc

import (
	stdhttp "net/http"

	"github.com/asynccnu/ccnubox-be/be-class_v2/conf"
	classgrpc "github.com/asynccnu/ccnubox-be/be-class_v2/grpc"
	classhttp "github.com/asynccnu/ccnubox-be/be-class_v2/http"
	bgrpc "github.com/asynccnu/ccnubox-be/common/bizpkg/grpc"
	grpcserver "github.com/asynccnu/ccnubox-be/common/bizpkg/grpc/server"
	"github.com/asynccnu/ccnubox-be/common/pkg/grpcx"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func InitGRPCServer(server *classgrpc.ClassServer, etcd *clientv3.Client, l logger.Logger, cfg *conf.InfraConf) grpcx.Server {
	return grpcserver.InitGRPCxKratosServer(server, etcd, l, (*cfg.Grpc)[bgrpc.CLASSS], cfg.Env)
}

func InitHTTPServer(handler *classhttp.Handler, cfg *conf.ServerConf) *stdhttp.Server {
	return &stdhttp.Server{Addr: cfg.Class.HTTP.Addr, Handler: handler}
}
