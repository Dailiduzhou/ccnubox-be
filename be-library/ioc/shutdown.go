package ioc

import (
	"context"
	"errors"

	clientv3 "go.etcd.io/etcd/client/v3"
	"gorm.io/gorm"
)

func InitShutdown(otelShutdown OTelShutdownFunc, db *gorm.DB, etcd *clientv3.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		// 即使 ctx 已取消，也要继续执行关闭操作，避免留下未关闭的资源
		var results []error
		if err := ctx.Err(); err != nil {
			results = append(results, err)
		}
		if sqlDB, err := db.DB(); err != nil {
			results = append(results, err)
		} else if err := sqlDB.Close(); err != nil {
			results = append(results, err)
		}
		if etcd != nil {
			if err := etcd.Close(); err != nil {
				results = append(results, err)
			}
		}
		// OTel 最后关闭，确保任务停止后待导出的 Span 能完成 flush
		if otelShutdown != nil {
			if err := otelShutdown(ctx); err != nil {
				results = append(results, err)
			}
		}
		return errors.Join(results...)
	}
}
