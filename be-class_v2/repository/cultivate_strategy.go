package repository

import (
	"context"
	"errors"
	"time"

	"github.com/asynccnu/ccnubox-be/be-class_v2/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class_v2/biz/usecase"
	"github.com/asynccnu/ccnubox-be/be-class_v2/conf"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRecordNotFound = usecase.ErrRecordNotFound
)

type CultivateStrategyData struct {
	db        *gorm.DB
	dataAlive time.Duration
}

func NewCultivateStrategyData(db *gorm.DB, c *conf.ServerConf) usecase.CultivateStrategyData {
	alive := time.Duration(c.Class.DataAliveDays) * 24 * time.Hour
	return &CultivateStrategyData{
		db:        db,
		dataAlive: alive,
	}
}

func (c *CultivateStrategyData) BatchSaveToBeStudiedClass(ctx context.Context,
	relations []model.UnStudiedClassStudentRelationship, classes []model.ToBeStudiedClass) error {
	tx := c.db.Begin()

	now := time.Now().Unix()
	for i := range relations {
		relations[i].UpdatedAt = now
	}
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "to_be_studied_class_id"}, {Name: "student_id"}}, // 这里联合差重
			DoUpdates: clause.AssignmentColumns([]string{"status", "updated_at"}),
		}).Create(&relations).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(&classes).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

func (c *CultivateStrategyData) GetClassStudentRelation(ctx context.Context, stuId, status string,
	dataAlive time.Duration) ([]model.UnStudiedClassStudentRelationship, error) {
	var result []model.UnStudiedClassStudentRelationship
	q := c.db.Model(&model.UnStudiedClassStudentRelationship{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if dataAlive > 0 {
		q = q.Where("updated_at > ?", time.Now().Add(-dataAlive))
	}

	if err := q.WithContext(ctx).Where("student_id = ?", stuId).Find(&result).Error; err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, ErrRecordNotFound
	}

	return result, nil
}

func (c *CultivateStrategyData) GetDetailUnStudyClass(ctx context.Context, id string) (model.ToBeStudiedClass, error) {
	var result model.ToBeStudiedClass
	if err := c.db.Model(&model.ToBeStudiedClass{}).WithContext(ctx).Where("id = ?", id).First(&result).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ToBeStudiedClass{}, ErrRecordNotFound
		}
		return model.ToBeStudiedClass{}, err
	}

	return result, nil
}

func (c *CultivateStrategyData) DataAlive() time.Duration {
	return c.dataAlive
}
