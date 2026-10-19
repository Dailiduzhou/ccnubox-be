package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/asynccnu/ccnubox-be/be-class/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class/biz/usecase"
	"github.com/asynccnu/ccnubox-be/be-class/conf"
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

func (c *CultivateStrategyData) getClassStudentRelation(ctx context.Context, stuId, status string,
	dataAlive time.Duration) ([]model.UnStudiedClassStudentRelationship, error) {
	var result []model.UnStudiedClassStudentRelationship
	q := c.db.Model(&model.UnStudiedClassStudentRelationship{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if dataAlive > 0 {
		q = q.Where("updated_at > ?", time.Now().Add(-dataAlive).Unix())
	}

	if err := q.WithContext(ctx).Where("student_id = ?", stuId).Find(&result).Error; err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, ErrRecordNotFound
	}

	return result, nil
}

func (c *CultivateStrategyData) ListStudentClasses(ctx context.Context, stuId, status string, dataAlive time.Duration) ([]model.ToBeStudiedClass, error) {
	relations, err := c.getClassStudentRelation(ctx, stuId, status, dataAlive)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(relations))
	seen := make(map[string]struct{}, len(relations))
	for _, relation := range relations {
		id := relation.ToBeStudiedClassId
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	const batchSize = 500
	details := make(map[string]model.ToBeStudiedClass, len(ids))
	for start := 0; start < len(ids); start += batchSize {
		end := min(start+batchSize, len(ids))
		var classes []model.ToBeStudiedClass
		if err := c.db.WithContext(ctx).Where("id IN ?", ids[start:end]).Find(&classes).Error; err != nil {
			return nil, err
		}
		for _, class := range classes {
			details[class.Id] = class
		}
	}

	result := make([]model.ToBeStudiedClass, 0, len(relations))
	for _, relation := range relations {
		class, ok := details[relation.ToBeStudiedClassId]
		if !ok {
			return nil, fmt.Errorf("%w: course %s has no detail", ErrRecordNotFound, relation.ToBeStudiedClassId)
		}
		class.Status = relation.Status
		result = append(result, class)
	}
	return result, nil
}

func (c *CultivateStrategyData) DataAlive() time.Duration {
	return c.dataAlive
}
