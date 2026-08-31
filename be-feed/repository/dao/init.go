package dao

import (
	"github.com/asynccnu/ccnubox-be/be-feed/repository/model"
	"gorm.io/gorm"
)

func InitTables(db *gorm.DB) error {
	// 创建用户配置表
	err := db.AutoMigrate(
		&model.FeedEvent{},
		&model.FeedUserConfig{},
		&model.FeedUserConfigChange{},
		&model.FeedUserToken{},
		&model.FeedFailEvent{},
		&model.FeedPushDelivery{},
	)
	if err != nil {
		return err
	}

	// 为内嵌 BaseModel 的表，手动创建 index
	if !db.Migrator().HasIndex(&model.FeedEvent{}, "idx_feed_events_inbox") {
		if err = db.Exec("CREATE INDEX idx_feed_events_inbox ON feed_events (student_id, deleted_at, created_at, id)").Error; err != nil {
			return err
		}
	}

	return nil
}
