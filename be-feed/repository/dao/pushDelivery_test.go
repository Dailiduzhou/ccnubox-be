package dao

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/asynccnu/ccnubox-be/be-feed/repository/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetPushFeedEventTreatsSoftDeletedEventAsNotFound(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&model.FeedEvent{}); err != nil {
		t.Fatalf("migrate feed event: %v", err)
	}
	event := model.FeedEvent{StudentId: "20260001", Type: "library", Title: "提醒"}
	if err = db.Create(&event).Error; err != nil {
		t.Fatalf("create feed event: %v", err)
	}
	if err = db.Delete(&event).Error; err != nil {
		t.Fatalf("soft delete feed event: %v", err)
	}

	repo := NewPushDeliveryDAO(db)
	_, err = repo.GetFeedEvent(context.Background(), event.ID)
	if !errors.Is(err, ErrFeedEventNotFound) {
		t.Fatalf("get soft-deleted feed event err=%v", err)
	}
}

func TestPushDeliveryPriority(t *testing.T) {
	for _, notificationType := range []string{"START_30", "END_10", "AWAY_60", "AWAY_80"} {
		event := model.FeedEvent{Type: "library", ExtendFields: model.ExtendFields{"notification_type": notificationType}}
		if priority := pushDeliveryPriority(event); priority <= 0 {
			t.Fatalf("notification type %s priority=%d", notificationType, priority)
		}
	}
	if priority := pushDeliveryPriority(model.FeedEvent{Type: "muxi"}); priority != 0 {
		t.Fatalf("normal message priority=%d", priority)
	}
}

func TestPushDeliveryListDuePrioritizesTimeSensitiveRows(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&model.FeedPushDelivery{}); err != nil {
		t.Fatalf("migrate push delivery: %v", err)
	}
	now := time.Now().Unix()
	rows := []model.FeedPushDelivery{
		{FeedEventID: 1, StudentId: "20260001", Status: model.PushDeliveryPending, Priority: 0, NextAttemptAt: now - 10},
		{FeedEventID: 2, StudentId: "20260002", Status: model.PushDeliveryPending, Priority: 100, NextAttemptAt: now},
		{FeedEventID: 3, StudentId: "20260003", Status: model.PushDeliveryPending, Priority: 100, NextAttemptAt: now - 1},
	}
	if err = db.Create(&rows).Error; err != nil {
		t.Fatalf("create push deliveries: %v", err)
	}

	due, err := NewPushDeliveryDAO(db).ListDue(context.Background(), now, len(rows))
	if err != nil {
		t.Fatalf("list due deliveries: %v", err)
	}
	want := []int64{rows[2].ID, rows[1].ID, rows[0].ID}
	if len(due) != len(want) {
		t.Fatalf("due count=%d, want=%d", len(due), len(want))
	}
	for i := range want {
		if due[i].ID != want[i] {
			t.Fatalf("due order=%v, want=%v", []int64{due[0].ID, due[1].ID, due[2].ID}, want)
		}
	}
}

func TestPushDeliveryRecoversSendingOnlyAtStartup(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&model.FeedPushDelivery{}); err != nil {
		t.Fatalf("migrate push delivery: %v", err)
	}
	repo := NewPushDeliveryDAO(db)
	ctx := context.Background()
	now := time.Now().Unix()
	rows := []model.FeedPushDelivery{
		{FeedEventID: 1, StudentId: "20260001", Status: model.PushDeliveryPending, NextAttemptAt: now},
		{FeedEventID: 2, StudentId: "20260002", Status: model.PushDeliverySending, Attempts: 2, NextAttemptAt: now},
	}
	if err = db.Create(&rows).Error; err != nil {
		t.Fatalf("create push deliveries: %v", err)
	}

	due, err := repo.ListDue(ctx, now, 10)
	if err != nil || len(due) != 1 || due[0].ID != rows[0].ID {
		t.Fatalf("due before recovery=%+v err=%v", due, err)
	}
	claimed, err := repo.Claim(ctx, rows[0].ID)
	if err != nil || !claimed {
		t.Fatalf("claim pending delivery: claimed=%v err=%v", claimed, err)
	}
	claimed, err = repo.Claim(ctx, rows[0].ID)
	if err != nil || claimed {
		t.Fatalf("claim sending delivery: claimed=%v err=%v", claimed, err)
	}

	if err = repo.RecoverSending(ctx); err != nil {
		t.Fatalf("recover sending deliveries: %v", err)
	}
	due, err = repo.ListDue(ctx, now, 10)
	if err != nil || len(due) != 2 || due[1].Attempts != 2 {
		t.Fatalf("due after recovery=%+v err=%v", due, err)
	}
	if err = repo.MarkSent(ctx, rows[0].ID); err == nil {
		t.Fatal("pending delivery was finalized without being claimed")
	}
}
