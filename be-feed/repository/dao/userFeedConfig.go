package dao

import (
	"context"
	"errors"

	"github.com/asynccnu/ccnubox-be/be-feed/repository/model"
	"github.com/asynccnu/ccnubox-be/common/pkg/errorx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FeedUserConfigDAO 用来对用户的feed数据进行处理
type FeedUserConfigDAO interface {
	FindOrCreateUserFeedConfig(ctx context.Context, studentId string) (*model.FeedUserConfig, error)
	SaveUserFeedConfig(ctx context.Context, req *model.FeedUserConfig) error
	SetConfigBit(config *uint16, position int)
	ClearConfigBit(config *uint16, position int)
	GetConfigBit(config uint16, position int) bool
	GetStudentIdsByCursor(ctx context.Context, lastID int64, limit int) ([]string, int64, error)
	ChangeConfigBits(ctx context.Context, studentID string, bits map[int]bool, library *bool) (*model.FeedUserConfig, error)
	IsLibraryEnabled(ctx context.Context, studentID string) (bool, error)
	ListLibraryPreferenceChanges(ctx context.Context, afterRevision int64, limit int) ([]model.FeedUserConfigChange, error)
	LatestLibraryPreferenceRevision(ctx context.Context) (int64, error)
	ListLibraryReminderUsers(ctx context.Context, afterID, snapshotRevision int64, limit int) ([]model.FeedUserConfig, error)
}

type feedUserConfigDAO struct {
	gorm *gorm.DB
}

// NewFeedUserConfigDAO 创建一个新的 FeedUserConfigDAO 实例
func NewFeedUserConfigDAO(db *gorm.DB) FeedUserConfigDAO {
	return &feedUserConfigDAO{gorm: db}
}

// FindOrCreateUserFeedConfig 查找或创建 FeedUserConfig
func (dao *feedUserConfigDAO) FindOrCreateUserFeedConfig(ctx context.Context, studentId string) (*model.FeedUserConfig, error) {
	allowList := model.FeedUserConfig{StudentId: studentId}
	err := dao.gorm.WithContext(ctx).Model(model.FeedUserConfig{}).
		Where("student_id = ?", studentId).
		FirstOrCreate(&allowList).Error
	if err != nil {
		return nil, errorx.Errorf("dao: find or create user feed config failed, sid: %s, err: %w", studentId, err)
	}
	return &allowList, nil
}

// SaveUserFeedConfig 保存 FeedUserConfig
func (dao *feedUserConfigDAO) SaveUserFeedConfig(ctx context.Context, req *model.FeedUserConfig) error {
	err := dao.gorm.WithContext(ctx).Save(req).Error
	if err != nil {
		return errorx.Errorf("dao: save user feed config failed, sid: %s, err: %w", req.StudentId, err)
	}
	return nil
}

// 设置指定位置的配置为 1
func (dao *feedUserConfigDAO) SetConfigBit(config *uint16, position int) {
	*config |= (1 << position)
}

// 设置指定位置的配置为 0
func (dao *feedUserConfigDAO) ClearConfigBit(config *uint16, position int) {
	*config &= ^(1 << position)
}

// 获取指定位置的配置值（返回 true 或 false）
func (dao *feedUserConfigDAO) GetConfigBit(config uint16, position int) bool {
	return (config & (1 << position)) != 0
}

func (dao *feedUserConfigDAO) GetStudentIdsByCursor(ctx context.Context, lastID int64, limit int) ([]string, int64, error) {
	var students []struct {
		ID        int64  `gorm:"column:id"`
		StudentId string `gorm:"column:student_id"`
	}

	query := dao.gorm.WithContext(ctx).Model(model.FeedUserConfig{}).
		Where("id > ?", lastID).
		Order("id ASC").
		Limit(limit)

	if err := query.Find(&students).Error; err != nil {
		return nil, 0, errorx.Errorf("dao: get student ids by cursor failed, lastID: %d, limit: %d, err: %w", lastID, limit, err)
	}

	if len(students) == 0 {
		return nil, 0, nil
	}

	var studentIds []string
	for _, student := range students {
		studentIds = append(studentIds, student.StudentId)
	}

	newLastID := students[len(students)-1].ID

	return studentIds, newLastID, nil
}

// 事务中修改 AllowList 的位图
func (dao *feedUserConfigDAO) ChangeConfigBits(
	ctx context.Context,
	studentID string,
	bits map[int]bool,
	library *bool,
) (*model.FeedUserConfig, error) {
	var config model.FeedUserConfig
	err := dao.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		config = model.FeedUserConfig{StudentId: studentID}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("student_id = ?", studentID).
			FirstOrCreate(&config).Error; err != nil {
			return err
		}

		for position, enabled := range bits {
			if enabled {
				config.PushConfig |= 1 << position
			} else {
				config.PushConfig &^= 1 << position
			}
		}

		libraryChanged := false
		if library != nil {
			wasEnabled := config.PushConfig&(1<<model.LibraryPos) != 0
			libraryChanged = wasEnabled != *library
			if *library {
				config.PushConfig |= 1 << model.LibraryPos
			} else {
				config.PushConfig &^= 1 << model.LibraryPos
			}
		}

		if libraryChanged {
			change := model.FeedUserConfigChange{
				StudentId:      studentID,
				LibraryEnabled: *library,
			}
			if err := tx.Create(&change).Error; err != nil {
				return err
			}
			config.LibraryRevision = change.Revision
		}
		return tx.Save(&config).Error
	})
	if err != nil {
		return nil, errorx.Errorf("dao: change feed config transaction failed, sid: %s, err: %w", studentID, err)
	}
	return &config, nil
}

func (dao *feedUserConfigDAO) IsLibraryEnabled(ctx context.Context, studentID string) (bool, error) {
	var config model.FeedUserConfig
	err := dao.gorm.WithContext(ctx).Where("student_id = ?", studentID).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errorx.Errorf("dao: get library preference failed, sid: %s, err: %w", studentID, err)
	}
	return config.PushConfig&(1<<model.LibraryPos) != 0, nil
}

func (dao *feedUserConfigDAO) ListLibraryPreferenceChanges(ctx context.Context, afterRevision int64, limit int) ([]model.FeedUserConfigChange, error) {
	var changes []model.FeedUserConfigChange
	err := dao.gorm.WithContext(ctx).
		Where("revision > ?", afterRevision).
		Order("revision ASC").
		Limit(limit).
		Find(&changes).Error
	if err != nil {
		return nil, errorx.Errorf("dao: list library preference changes failed, revision: %d, err: %w", afterRevision, err)
	}
	return changes, nil
}

// 全量分页开始前固定变更水位，水位之后的用户由增量同步重放。
func (dao *feedUserConfigDAO) LatestLibraryPreferenceRevision(ctx context.Context) (int64, error) {
	var revision int64
	err := dao.gorm.WithContext(ctx).
		Model(&model.FeedUserConfigChange{}).
		Select("COALESCE(MAX(revision), 0) AS revision").
		Scan(&revision).Error
	if err != nil {
		return 0, errorx.Errorf("dao: get latest library preference revision failed, err: %w", err)
	}
	return revision, nil
}

// 只返回水位内未发生后续变更的用户，避免实时 enabled 集合在翻页期间前移。
func (dao *feedUserConfigDAO) ListLibraryReminderUsers(ctx context.Context, afterID, snapshotRevision int64, limit int) ([]model.FeedUserConfig, error) {
	var configs []model.FeedUserConfig
	err := dao.gorm.WithContext(ctx).
		Where("id > ? AND library_revision <= ? AND (push_config & ?) <> 0", afterID, snapshotRevision, uint16(1<<model.LibraryPos)).
		Order("id ASC").
		Limit(limit).
		Find(&configs).Error
	if err != nil {
		return nil, errorx.Errorf("dao: list library reminder users failed, id: %d, snapshot revision: %d, err: %w", afterID, snapshotRevision, err)
	}
	return configs, nil
}
