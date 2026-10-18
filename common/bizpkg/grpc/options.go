package grpc

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/asynccnu/ccnubox-be/common/bizpkg/conf"
)

type GRPCServerOptions struct {
	Name    string
	Addr    string
	Weight  int
	EtcdTTL time.Duration
	Timeout time.Duration
}

type GRPCClientOptions struct {
	Name    string
	Timeout time.Duration
}

// GetConf 从共享服务目录取配置，缺项在启动时明确失败。
func GetConf(cfg *conf.GrpcConfs, key string) *conf.GrpcConf {
	if cfg == nil || (*cfg)[key] == nil {
		panic(fmt.Sprintf("gRPC 配置中缺失服务定义: %s", key))
	}
	c := (*cfg)[key]
	if err := validateName(c); err != nil {
		panic(fmt.Sprintf("gRPC 服务 %s 配置错误: %v", key, err))
	}
	return c
}

func ClientOptionsFrom(cfg *conf.GrpcConf) (GRPCClientOptions, error) {
	if err := validateName(cfg); err != nil {
		return GRPCClientOptions{}, err
	}
	timeout, err := configDuration(cfg.ClientTimeout, time.Second, "clientTimeout")
	if err != nil {
		return GRPCClientOptions{}, err
	}
	return GRPCClientOptions{Name: cfg.Name, Timeout: timeout}, nil
}

func ServerOptionsFrom(cfg *conf.GrpcConf) (GRPCServerOptions, error) {
	if err := validateName(cfg); err != nil {
		return GRPCServerOptions{}, err
	}
	_, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return GRPCServerOptions{}, fmt.Errorf("invalid gRPC addr %q: %w", cfg.Addr, err)
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 0 || p > 65535 {
		return GRPCServerOptions{}, fmt.Errorf("invalid gRPC port %q", port)
	}
	if cfg.Weight < 0 || cfg.EtcdTTL <= 0 {
		return GRPCServerOptions{}, fmt.Errorf("gRPC weight must be non-negative and etcdTTL must be positive")
	}
	ttl, err := configDuration(cfg.EtcdTTL, time.Second, "etcdTTL")
	if err != nil {
		return GRPCServerOptions{}, err
	}
	timeout, err := configDuration(cfg.ServerTimeout, time.Minute, "serverTimeout")
	if err != nil {
		return GRPCServerOptions{}, err
	}
	return GRPCServerOptions{
		Name: cfg.Name, Addr: cfg.Addr, Weight: cfg.Weight, EtcdTTL: ttl, Timeout: timeout,
	}, nil
}

func validateName(cfg *conf.GrpcConf) error {
	if cfg == nil {
		return fmt.Errorf("gRPC configuration is nil")
	}
	if cfg.Name == "" || strings.TrimSpace(cfg.Name) != cfg.Name {
		return fmt.Errorf("gRPC service name is empty or has surrounding whitespace")
	}
	return nil
}

// 保留旧配置单位及零超时语义，避免拆分配置时改变请求超时。
func configDuration(n int, unit time.Duration, field string) (time.Duration, error) {
	if n < 0 || int64(n) > (1<<63-1)/int64(unit) {
		return 0, fmt.Errorf("invalid gRPC %s: %d", field, n)
	}
	return time.Duration(n) * unit, nil
}
