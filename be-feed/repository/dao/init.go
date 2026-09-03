package dao

import (
	"github.com/asynccnu/ccnubox-be/be-feed/repository/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// feedEventDedupeKeyMigration 仅用于第一阶段迁移，保证新增列允许为空且不创建索引。
type feedEventDedupeKeyMigration struct {
	DedupeKey *string `gorm:"column:dedupe_key;type:varchar(255)"`
}

func (feedEventDedupeKeyMigration) TableName() string {
	return "feed_events"
}

func InitTables(db *gorm.DB) error {
	if err := migrateFeedEventDedupeKey(db); err != nil {
		return err
	}
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
	if err = migrateInitialLibraryPreference(db); err != nil {
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

// migrateFeedEventDedupeKey 分阶段增加去重键，避免在历史数据回填前收紧列约束或创建唯一索引。
func migrateFeedEventDedupeKey(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable(&model.FeedEvent{}) {
		return nil
	}

	// 第一阶段先增加可空列，不能直接使用带 NOT NULL 和唯一索引的 FeedEvent。
	if !migrator.HasColumn(&model.FeedEvent{}, "DedupeKey") {
		if err := migrator.AddColumn(&feedEventDedupeKeyMigration{}, "DedupeKey"); err != nil {
			return err
		}
	}

	columnTypes, err := migrator.ColumnTypes(&model.FeedEvent{})
	if err != nil {
		return err
	}
	needsNotNull := true
	for _, columnType := range columnTypes {
		if columnType.Name() != "dedupe_key" {
			continue
		}
		if nullable, ok := columnType.Nullable(); ok && !nullable {
			needsNotNull = false
		}
		break
	}
	hasRecipientIndex := migrator.HasIndex(&model.FeedEvent{}, "uidx_feed_events_recipient_dedupe")
	hasLegacyIndex := migrator.HasIndex(&model.FeedEvent{}, "uidx_feed_events_dedupe")
	if !needsNotNull && hasRecipientIndex && !hasLegacyIndex {
		return nil
	}

	// 第二阶段按主键生成稳定 ID，单条语句即可完成，避免大表逐行回填拖慢启动。
	if err = db.Unscoped().Exec(
		"UPDATE feed_events SET dedupe_key = CONCAT('legacy:', id) WHERE dedupe_key IS NULL OR dedupe_key = ''",
	).Error; err != nil {
		return err
	}

	// 第三阶段仅在列仍可空时收紧约束，避免每次启动都重复执行 DDL。
	if needsNotNull {
		if err = migrator.AlterColumn(&model.FeedEvent{}, "DedupeKey"); err != nil {
			return err
		}
	}

	// 第四阶段在回填并收紧约束后创建接收者维度的唯一索引。
	if !hasRecipientIndex {
		if err = migrator.CreateIndex(&model.FeedEvent{}, "uidx_feed_events_recipient_dedupe"); err != nil {
			return err
		}
	}

	// 第五阶段显式删除旧索引；AutoMigrate 不会删除模型中已移除的索引。
	if hasLegacyIndex {
		if err = migrator.DropIndex(&model.FeedEvent{}, "uidx_feed_events_dedupe"); err != nil {
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

// 首次迁移图书馆开关时，将尚无偏好版本的已有用户默认设为开启。
func migrateInitialLibraryPreference(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.FeedUserConfig{}).
			Where("library_revision = 0").
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}

		var allocator model.FeedUserConfigRevisionAllocator
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&allocator, model.FeedUserConfigRevisionAllocatorID).Error; err != nil {
			return err
		}
		if allocator.Revision == 0 {
			allocator.Revision = 1
			if err := tx.Model(&allocator).UpdateColumn("revision", allocator.Revision).Error; err != nil {
				return err
			}
		}

		return tx.Model(&model.FeedUserConfig{}).
			Where("library_revision = 0").
			UpdateColumns(map[string]any{
				"push_config":      gorm.Expr("push_config | ?", uint16(1<<model.LibraryPos)),
				"library_revision": allocator.Revision,
			}).Error
	})
}
