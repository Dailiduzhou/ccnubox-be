package repository

import (
	"context"
	"time"

	"github.com/asynccnu/ccnubox-be/be-content/repository/model"
	"github.com/asynccnu/ccnubox-be/common/pkg/errorx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CalendarRepo extends the generic content repository with an upsert that can
// restore a soft-deleted calendar occupying the unique year index.
type CalendarRepo interface {
	ContentRepo[model.Calendar]
	Upsert(ctx context.Context, calendar *model.Calendar) error
}

type calendarRepository struct {
	*Repository[model.Calendar]
	db *gorm.DB
}

func (r *calendarRepository) Upsert(ctx context.Context, calendar *model.Calendar) error {
	now := time.Now()
	calendar.UpdatedAt = now

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "year"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"link":       calendar.Link,
			"deleted_at": nil,
			"updated_at": now,
		}),
	}).Create(calendar).Error
	if err != nil {
		return errorx.Errorf("upsert calendar failed: %w, year: %d", err, calendar.Year)
	}

	if err := r.cache.ClearContent(ctx); err != nil {
		return errorx.Errorf("clear calendar cache failed: %w", err)
	}
	return nil
}
