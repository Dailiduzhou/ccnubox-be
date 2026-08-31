package dao

import (
	"github.com/asynccnu/ccnubox-be/be-feed/repository/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func InitTables(db *gorm.DB) error {
	// 创建用户配置表
	err := db.AutoMigrate(
		&model.FeedEvent{},
		&model.FeedUserConfig{},
		&model.FeedUserConfigChange{},
		&model.FeedUserConfigRevisionAllocator{},
		&model.FeedUserToken{},
		&model.FeedFailEvent{},
		&model.FeedPushDelivery{},
	)
	if err != nil {
		return err
	}
	if err = initFeedUserConfigRevisionAllocator(db); err != nil {
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

// 首次升级时从已有变更记录衔接版本号，后续启动只允许分配器向前校正。
func initFeedUserConfigRevisionAllocator(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var allocator model.FeedUserConfigRevisionAllocator
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Find(&allocator, model.FeedUserConfigRevisionAllocatorID).Error
		if err != nil {
			return err
		}

		var latest int64
		if err = tx.Model(&model.FeedUserConfigChange{}).
			Select("COALESCE(MAX(revision), 0)").
			Scan(&latest).Error; err != nil {
			return err
		}
		if allocator.ID == 0 {
			allocator = model.FeedUserConfigRevisionAllocator{
				ID:       model.FeedUserConfigRevisionAllocatorID,
				Revision: latest,
			}
			return tx.Create(&allocator).Error
		}
		if allocator.Revision < latest {
			return tx.Model(&allocator).UpdateColumn("revision", latest).Error
		}
		return nil
	})
}
