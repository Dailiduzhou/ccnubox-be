package ioc

import (
	"github.com/asynccnu/ccnubox-be/be-class_v2/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class_v2/conf"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/infra"
	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gorm.io/gorm"
)

func InitDB(cfg *conf.InfraConf) *gorm.DB {
	db := infra.InitMysql(cfg.Mysql)
	if err := db.AutoMigrate(&model.UnStudiedClassStudentRelationship{}, &model.ToBeStudiedClass{}); err != nil {
		panic(err)
	}
	return db
}

func InitRedis(cfg *conf.InfraConf) *redis.Client         { return infra.InitRedis(cfg.Redis) }
func InitEtcdClient(cfg *conf.InfraConf) *clientv3.Client { return infra.InitEtcdClient(cfg.Etcd) }
