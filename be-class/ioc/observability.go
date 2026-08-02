package ioc

import (
	"context"

	"github.com/asynccnu/ccnubox-be/be-class/conf"
	bgrpc "github.com/asynccnu/ccnubox-be/common/bizpkg/grpc"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/otel"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
)

func InitMetrics() *metricsx.Metrics { return metricsx.New("ccnubox") }

func InitMetricsServer(cfg *conf.InfraConf) *metricsx.Server {
	grpcCfg := (*cfg.Grpc)[bgrpc.CLASSS]
	if grpcCfg == nil {
		return metricsx.NewServer("")
	}
	return metricsx.NewServerFromGRPCAddr(grpcCfg.Addr)
}

func InitOTel(cfg *conf.InfraConf) func(context.Context) error {
	return otel.InitOTelFromInfra(cfg.InfraConf, bgrpc.CLASSS)
}
