package conf

import (
	"fmt"
	"strings"
	"time"
)

// 内部运行配置与旧 YAML 结构分离，监听及注册参数不再传给客户端。
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

func (c *GrpcConfs) Get(key string) *GrpcConf {
	if c == nil || (*c)[key] == nil {
		panic(fmt.Sprintf("gRPC 配置中缺失服务定义: %s", key))
	}
	cfg := (*c)[key]
	if strings.TrimSpace(cfg.Name) == "" {
		panic(fmt.Sprintf("gRPC 服务 %s 的名称不能为空", key))
	}
	return cfg
}

func (c *GrpcConf) ClientOptions() (GRPCClientOptions, error) {
	if c == nil {
		return GRPCClientOptions{}, fmt.Errorf("gRPC 客户端配置不能为空")
	}
	timeout, err := grpcDuration(c.ClientTimeout, time.Second)
	if err != nil {
		return GRPCClientOptions{}, fmt.Errorf("gRPC 客户端 %s 的 clientTimeout: %w", c.Name, err)
	}
	opts := GRPCClientOptions{Name: c.Name, Timeout: timeout}
	return opts, opts.Validate()
}

func (c *GrpcConf) ServerOptions() (GRPCServerOptions, error) {
	if c == nil {
		return GRPCServerOptions{}, fmt.Errorf("gRPC 服务端配置不能为空")
	}
	ttl, err := grpcDuration(c.EtcdTTL, time.Second)
	if err != nil {
		return GRPCServerOptions{}, fmt.Errorf("gRPC 服务端 %s 的 etcdTTL: %w", c.Name, err)
	}
	// 保留旧配置的分钟单位，0 继续表示不设置服务端超时。
	timeout, err := grpcDuration(c.ServerTimeout, time.Minute)
	if err != nil {
		return GRPCServerOptions{}, fmt.Errorf("gRPC 服务端 %s 的 serverTimeout: %w", c.Name, err)
	}
	opts := GRPCServerOptions{Name: c.Name, Addr: c.Addr, Weight: c.Weight, EtcdTTL: ttl, Timeout: timeout}
	return opts, opts.Validate()
}

func (o GRPCClientOptions) Validate() error {
	if strings.TrimSpace(o.Name) == "" || o.Timeout < 0 {
		return fmt.Errorf("gRPC 客户端 %q 的名称不能为空，超时不能为负数", o.Name)
	}
	return nil
}

func (o GRPCServerOptions) Validate() error {
	if strings.TrimSpace(o.Name) == "" || strings.TrimSpace(o.Addr) == "" {
		return fmt.Errorf("gRPC 服务端 %q 的名称和监听地址不能为空", o.Name)
	}
	if o.EtcdTTL < time.Second || o.Timeout < 0 {
		return fmt.Errorf("gRPC 服务端 %s 的 etcdTTL 至少为一秒，超时不能为负数", o.Name)
	}
	return nil
}

func grpcDuration(value int, unit time.Duration) (time.Duration, error) {
	if value < 0 || uint64(value) > uint64(1<<63-1)/uint64(unit) {
		return 0, fmt.Errorf("时长越界: %d", value)
	}
	return time.Duration(value) * unit, nil
}
