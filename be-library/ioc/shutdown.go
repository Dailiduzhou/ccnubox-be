package ioc

import (
	"context"
	"errors"

	clientv3 "go.etcd.io/etcd/client/v3"
	"gorm.io/gorm"
)

func InitShutdown(db *gorm.DB, etcd *clientv3.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var results []error
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
		return errors.Join(results...)
	}
}
