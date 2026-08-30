package ioc

import (
	"github.com/asynccnu/ccnubox-be/be-classlist/conf"
	contentv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/content/v1"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/grpc/client"
	etcdv3 "go.etcd.io/etcd/client/v3"
)

func InitContentSvcClient(etcdClient *etcdv3.Client, cfg *conf.InfraConf) contentv1.ContentServiceClient {
	return client.InitContent(etcdClient, cfg.Grpc, cfg.Env)
}
