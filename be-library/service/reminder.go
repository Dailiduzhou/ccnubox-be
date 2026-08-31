package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/asynccnu/ccnubox-be/be-library/conf"
	"github.com/asynccnu/ccnubox-be/be-library/crawler"
	"github.com/asynccnu/ccnubox-be/be-library/repository/dao"
	feedv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/feed/v1"
	userv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/user/v1"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"gorm.io/gorm"
)

const (
	NotificationReservationDiscovered = "RESERVATION_DISCOVERED"
	NotificationStart30               = "START_30"
	NotificationEnd10                 = "END_10"
	NotificationAway60                = "AWAY_60"
	NotificationAway80                = "AWAY_80"
	NotificationBreach                = "BREACH"
	NotificationBlacklisted           = "BLACKLISTED"
)

type notificationPayload struct {
	NotificationType string `json:"notification_type"`
	ReservationID    string `json:"reservation_id,omitempty"`
	SeatID           string `json:"seat_id,omitempty"`
	SeatLabel        string `json:"seat_label,omitempty"`
	Location         string `json:"location,omitempty"`
	StartAt          int64  `json:"start_at,omitempty"`
	EndAt            int64  `json:"end_at,omitempty"`
	TargetAt         int64  `json:"target_at,omitempty"`
	EpisodeVersion   int    `json:"episode_version,omitempty"`
	Message          string `json:"message,omitempty"`
}

type userTaskKey struct {
	studentID string
	taskType  string
}

type awayObservation struct {
	isAway         bool
	elapsedMinutes int
	elapsedKnown   bool
}

type userTaskState struct {
	preferenceVersion int64
	generation        int64
	running           bool
	nextAllowed       time.Time
}

type userTaskGate struct {
	mu     sync.Mutex
	states map[userTaskKey]userTaskState
}

// userTaskFinishFunc 用于结束一次已获得执行许可的用户任务。
// success 为 true 时保留最小执行间隔，为 false 时立即允许重试。
type userTaskFinishFunc func(success bool)

func newUserTaskGate() *userTaskGate {
	return &userTaskGate{states: make(map[userTaskKey]userTaskState)}
}

// start 串行化同一用户的同类任务，并在成功后保留最小执行间隔。
// 偏好版本变化时立即使用新状态，旧任务的结束回调不会覆盖它。
func (g *userTaskGate) start(studentID, taskType string, preferenceVersion int64, now time.Time, interval time.Duration) (userTaskFinishFunc, bool) {
	key := userTaskKey{studentID: studentID, taskType: taskType}
	g.mu.Lock()
	state := g.states[key]
	if state.preferenceVersion == preferenceVersion && (state.running || now.Before(state.nextAllowed)) {
		g.mu.Unlock()
		return nil, false
	}
	state.preferenceVersion = preferenceVersion
	state.generation++
	state.running = true
	state.nextAllowed = now.Add(interval)
	g.states[key] = state
	generation := state.generation
	g.mu.Unlock()

	return func(success bool) {
		g.mu.Lock()
		defer g.mu.Unlock()
		current, ok := g.states[key]
		if !ok || current.generation != generation || current.preferenceVersion != preferenceVersion {
			return
		}
		if !success || interval <= 0 {
			delete(g.states, key)
			return
		}
		current.running = false
		g.states[key] = current
	}, true
}

type ReminderService struct {
	dao          *dao.ReminderDAO
	crawler      crawler.ReminderCrawler
	user         userv1.UserServiceClient
	feed         FeedGateway
	config       conf.LibraryReminderConf
	logger       logger.Logger
	metrics      *metricsx.LibraryReminderMetrics
	now          func() time.Time
	preferenceMu *sync.Mutex
	userTaskGate *userTaskGate
}

func NewReminderService(repo *dao.ReminderDAO, reminderCrawler crawler.ReminderCrawler, user userv1.UserServiceClient, feed FeedGateway, serverConf *conf.ServerConf, metricSet *metricsx.Metrics, l logger.Logger) *ReminderService {
	var reminderMetrics *metricsx.LibraryReminderMetrics
	if metricSet != nil {
		reminderMetrics = metricSet.Library
	}
	return &ReminderService{dao: repo, crawler: reminderCrawler, user: user, feed: feed, config: serverConf.Reminder(), logger: l, metrics: reminderMetrics, now: time.Now, preferenceMu: &sync.Mutex{}, userTaskGate: newUserTaskGate()}
}

func (s *ReminderService) Enabled() bool { return s.config.Enabled }

func (s *ReminderService) RecoverOrphanedWork(ctx context.Context) error {
	return s.dao.RecoverOrphanedWork(ctx)
}

func (s *ReminderService) SuppressDisabledWork(ctx context.Context) error {
	return s.dao.SuppressAllWork(ctx)
}

func (s *ReminderService) SyncPreferences(ctx context.Context) (err error) {
	defer func() {
		if s.metrics == nil {
			return
		}
		result := "success"
		if err != nil {
			result = "error"
		}
		s.metrics.PreferenceSyncTotal.WithLabelValues(result).Inc()
	}()
	s.preferenceMu.Lock()
	defer s.preferenceMu.Unlock()
	return s.syncPreferences(ctx)
}

func (s *ReminderService) syncPreferences(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	cursor, err := s.dao.Cursor(ctx)
	if err != nil {
		return err
	}
	if cursor < 0 {
		if err := s.rebuildPreferences(ctx); err != nil {
			return err
		}
		cursor, err = s.dao.Cursor(ctx)
		if err != nil {
			return err
		}
	}
	for {
		callCtx, cancel := s.remoteCallContext(ctx)
		defer cancel()
		changes, next, err := s.feed.PreferenceChanges(callCtx, cursor, 200)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			return s.refreshPendingBaselines(ctx)
		}
		daoChanges := make([]dao.PreferenceChange, 0, len(changes))
		lastRevision := cursor
		for _, change := range changes {
			if change.StudentID == "" || len(change.StudentID) > 64 || change.Revision <= lastRevision {
				return errors.New("feed returned an invalid library preference change")
			}
			daoChanges = append(daoChanges, dao.PreferenceChange{Revision: change.Revision, StudentID: change.StudentID, Enabled: change.Enabled})
			lastRevision = change.Revision
			if s.metrics != nil && change.ChangedAt > 0 {
				lag := s.now().Unix() - change.ChangedAt
				if lag < 0 {
					lag = 0
				}
				s.metrics.PreferenceSyncLagSeconds.Set(float64(lag))
			}
		}
		if next != lastRevision {
			return errors.New("feed preference cursor does not match the last change")
		}
		enabled, err := s.dao.ApplyPreferenceChanges(ctx, daoChanges, next)
		if err != nil {
			return err
		}
		if s.config.ShouldBaselineOnEnable() {
			if err := s.forEachSubscription(ctx, enabled, s.RefreshUser); err != nil {
				s.logger.Warn("library reminder enable baseline batch failed", logger.Error(err))
			}
		}
		cursor = next
	}
}

func (s *ReminderService) rebuildPreferences(ctx context.Context) error {
	all, revision, err := s.loadReminderUsers(ctx)
	if err != nil {
		return err
	}
	enabled, err := s.dao.RebuildPreferences(ctx, all, revision)
	if err != nil {
		return err
	}
	if s.config.ShouldBaselineOnEnable() {
		if err := s.forEachSubscription(ctx, enabled, s.RefreshUser); err != nil {
			s.logger.Warn("library reminder rebuilt baseline batch failed", logger.Error(err))
		}
	}
	return nil
}

func (s *ReminderService) refreshPendingBaselines(ctx context.Context) error {
	if !s.config.ShouldBaselineOnEnable() {
		return nil
	}
	pending, err := s.dao.PendingBaselineSubscriptions(ctx, 100000)
	if err != nil || len(pending) == 0 {
		return err
	}
	if err := s.forEachSubscription(ctx, pending, s.RefreshUser); err != nil {
		s.logger.Warn("library reminder pending baseline batch failed", logger.Error(err))
	}
	return nil
}

// 凌晨 3:15 全量校准 Feed 偏好
func (s *ReminderService) CalibratePreferences(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	s.preferenceMu.Lock()
	defer s.preferenceMu.Unlock()
	all, _, err := s.loadReminderUsers(ctx)
	if err != nil {
		return err
	}
	enabled, err := s.dao.CalibratePreferences(ctx, all)
	if err != nil {
		return err
	}
	if s.config.ShouldBaselineOnEnable() {
		if err := s.forEachSubscription(ctx, enabled, s.RefreshUser); err != nil {
			s.logger.Warn("library reminder calibrated baseline batch failed", logger.Error(err))
		}
	}
	// 释放互斥锁前重放与分页全量查询并发的变更，消除全量快照与增量变更的顺序间隙。
	return s.syncPreferences(ctx)
}

func (s *ReminderService) loadReminderUsers(ctx context.Context) ([]dao.PreferenceChange, int64, error) {
	after := int64(0)
	snapshotRevision := int64(0)
	firstPage := true
	var all []dao.PreferenceChange
	for {
		callCtx, cancel := s.remoteCallContext(ctx)
		users, next, pageSnapshotRevision, err := s.feed.ReminderUsers(callCtx, after, snapshotRevision, 200)
		cancel()
		if err != nil {
			return nil, 0, err
		}
		if pageSnapshotRevision < 0 || (!firstPage && pageSnapshotRevision != snapshotRevision) {
			return nil, 0, errors.New("feed reminder user snapshot revision changed during pagination")
		}
		snapshotRevision = pageSnapshotRevision
		firstPage = false
		for _, user := range users {
			if user.StudentID == "" || len(user.StudentID) > 64 || user.Revision <= 0 || user.Revision > snapshotRevision {
				return nil, 0, errors.New("feed returned an invalid library reminder user")
			}
			all = append(all, dao.PreferenceChange{Revision: user.Revision, StudentID: user.StudentID, Enabled: true})
		}
		if len(users) == 0 {
			break
		}
		if next <= after {
			return nil, 0, errors.New("feed reminder user cursor did not advance")
		}
		after = next
	}
	return all, snapshotRevision, nil
}

func (s *ReminderService) RefreshAll(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	rows, err := s.dao.EnabledSubscriptions(ctx, 100000)
	if err != nil {
		return err
	}
	return s.forEachSubscription(ctx, rows, s.RefreshUser)
}

func (s *ReminderService) RefreshUser(ctx context.Context, sub dao.LibraryReminderSubscription) (err error) {
	defer func() {
		if s.metrics == nil {
			return
		}
		result := "success"
		if err != nil {
			result = "error"
		}
		s.metrics.RefreshUsersTotal.WithLabelValues(result).Inc()
	}()
	current, err := s.dao.Subscription(ctx, sub.StudentID)
	if err != nil || !current.Enabled || current.PreferenceVersion != sub.PreferenceVersion {
		if err != nil {
			return err
		}
		return nil
	}
	finish, started := s.userTaskGate.start(sub.StudentID, "full_refresh", sub.PreferenceVersion, s.now(), s.config.FullRefreshMinInterval)
	if !started {
		return nil
	}
	defer func() { finish(err == nil) }()
	token, err := s.libraryToken(ctx, sub.StudentID)
	if err != nil {
		_ = s.dao.MarkRefreshFailure(ctx, sub.StudentID, "AUTH_ERROR")
		return err
	}
	if current, err := s.subscriptionStillCurrent(ctx, sub); err != nil || !current {
		return err
	}
	today, err := s.crawler.GetTodayReservations(ctx, token)
	if err != nil {
		_ = s.dao.MarkRefreshFailure(ctx, sub.StudentID, "UPSTREAM_UNKNOWN")
		return err
	}
	if current, err := s.subscriptionStillCurrent(ctx, sub); err != nil || !current {
		return err
	}
	watermark, err := s.dao.LatestReservationID(ctx, sub.StudentID)
	if err != nil {
		return err
	}
	history, err := s.crawler.GetRecentHistory(ctx, token, crawler.HistoryWatermark{ReservationID: watermark})
	if err != nil {
		_ = s.dao.MarkRefreshFailure(ctx, sub.StudentID, "UPSTREAM_UNKNOWN")
		return err
	}
	if current, err := s.subscriptionStillCurrent(ctx, sub); err != nil || !current {
		return err
	}
	state, err := s.crawler.GetUserState(ctx, token)
	if err != nil {
		_ = s.dao.MarkRefreshFailure(ctx, sub.StudentID, "UPSTREAM_UNKNOWN")
		return err
	}
	if s.config.NotificationTypes.Breach && state.BreachNum > 0 {
		if current, err := s.subscriptionStillCurrent(ctx, sub); err != nil || !current {
			return err
		}
		if _, err := s.crawler.GetBreaches(ctx, token, 0, s.config.HistoryPageSize); err != nil {
			_ = s.dao.MarkRefreshFailure(ctx, sub.StudentID, "UPSTREAM_UNKNOWN")
			return err
		}
	}
	// 部分历史分页可安全用于更新插入和新增事实，但不能据此执行“缺失即取消”的状态转换。
	reservations := make(map[string]crawler.ReminderReservation, len(today)+len(history.Reservations))
	for _, row := range history.Reservations {
		reservations[row.ID] = row
	}
	for _, row := range today {
		reservations[row.ID] = row
	}
	now := s.now()
	return s.dao.Transaction(ctx, func(txDAO *dao.ReminderDAO) error {
		locked, err := txDAO.SubscriptionForUpdate(ctx, sub.StudentID)
		if err != nil {
			return err
		}
		if !locked.Enabled || locked.PreferenceVersion != sub.PreferenceVersion {
			return nil
		}
		txService := *s
		txService.dao = txDAO
		for _, row := range reservations {
			if err := txService.reconcileReservation(ctx, *locked, row, now); err != nil {
				return err
			}
		}
		if err := txService.reconcileUserState(ctx, *locked, state, now); err != nil {
			return err
		}
		return txDAO.MarkBaseline(ctx, sub.StudentID, now, "OK")
	})
}

func (s *ReminderService) reconcileReservation(ctx context.Context, sub dao.LibraryReminderSubscription, row crawler.ReminderReservation, now time.Time) error {
	start, end, err := row.Times()
	if err != nil {
		return err
	}
	status := strings.ToUpper(strings.TrimSpace(row.Status))
	if status != "" && !terminalReservationStatus(status) && status != "USING" && status != "WAIT" && status != "SUCCESS" {
		s.logger.Warn("unknown library reservation status", logger.String("student_id", maskStudentID(sub.StudentID)), logger.String("status", status))
		if s.metrics != nil {
			s.metrics.UnknownReservationStatusTotal.Inc()
		}
	}
	snapshot := &dao.ReservationSnapshot{StudentID: sub.StudentID, ExternalReservationID: row.ID, SeatID: row.SeatID, SeatLabel: row.SeatLabel, Location: row.Location, MakeDate: row.MakeDateStr, StartAt: start, EndAt: end, Status: status, RawStatus: row.Status, FirstSeenAt: now, LastSeenAt: now}
	created, err := s.dao.SaveReservation(ctx, snapshot)
	if err != nil {
		return err
	}
	if terminalReservationStatus(status) {
		return s.dao.CancelReservationJobs(ctx, sub.StudentID, row.ID)
	}
	if shouldEnqueueReservationDiscovered(created, sub.BaselineCompleted, end, now, s.config.NotificationTypes.ReservationDiscovered) {
		if err := s.enqueueFact(ctx, sub, NotificationReservationDiscovered, row.ID, 0, start, end, now); err != nil {
			return err
		}
	}
	if s.config.NotificationTypes.Start30 && start.After(now) {
		runAt := start.Add(-30 * time.Minute)
		if runAt.Before(now) {
			runAt = now
		}
		key := reminderJobKey(sub.StudentID, row.ID, NotificationStart30, start.Unix())
		if err := s.dao.CancelStaleReservationJobType(ctx, sub.StudentID, row.ID, NotificationStart30, key); err != nil {
			return err
		}
		if err := s.scheduleJob(ctx, sub, NotificationStart30, row.ID, 0, runAt, start.Unix()); err != nil {
			return err
		}
	} else if err := s.dao.CancelReservationJobTypes(ctx, sub.StudentID, row.ID, []string{NotificationStart30}); err != nil {
		return err
	}
	if s.config.NotificationTypes.End10 && end.After(now) {
		runAt := end.Add(-10 * time.Minute)
		if runAt.Before(now) {
			runAt = now
		}
		key := reminderJobKey(sub.StudentID, row.ID, NotificationEnd10, end.Unix())
		if err := s.dao.CancelStaleReservationJobType(ctx, sub.StudentID, row.ID, NotificationEnd10, key); err != nil {
			return err
		}
		if err := s.scheduleJob(ctx, sub, NotificationEnd10, row.ID, 0, runAt, end.Unix()); err != nil {
			return err
		}
	} else if err := s.dao.CancelReservationJobTypes(ctx, sub.StudentID, row.ID, []string{NotificationEnd10}); err != nil {
		return err
	}
	return nil
}

func shouldEnqueueReservationDiscovered(created, baselineCompleted bool, end, now time.Time, enabled bool) bool {
	return enabled && created && baselineCompleted && end.After(now)
}

func (s *ReminderService) reconcileUserState(ctx context.Context, sub dao.LibraryReminderSubscription, state crawler.LibraryUserState, now time.Time) error {
	row := &dao.LibraryUserStateSnapshot{StudentID: sub.StudentID, CycleTimeName: state.CycleTimeName, BreachNum: state.BreachNum, ScoreNum: state.ScoreNum, BlackTime: state.BlackTime, BlackMessage: state.BlackMessage, LastSeenAt: now}
	previous, err := s.dao.SaveUserState(ctx, row)
	if err != nil || previous == nil || !sub.BaselineCompleted {
		return err
	}
	if s.config.NotificationTypes.Breach && state.CycleTimeName == previous.CycleTimeName && state.BreachNum > previous.BreachNum {
		key := boundedDedupeKey(fmt.Sprintf("%s:%s:%d:%s", sub.StudentID, state.CycleTimeName, state.BreachNum, NotificationBreach))
		payload := notificationPayload{NotificationType: NotificationBreach, TargetAt: now.Unix()}
		if err := s.enqueuePayload(ctx, sub, key, NotificationBreach, payload); err != nil {
			return err
		}
	}
	enteredBlacklist := (previous.BlackTime == "" && state.BlackTime != "") || (previous.BlackMessage == "" && state.BlackMessage != "") || (state.BlackTime != "" && state.BlackTime != previous.BlackTime)
	if s.config.NotificationTypes.Blacklisted && enteredBlacklist {
		key := boundedDedupeKey(fmt.Sprintf("%s:%s:%s", sub.StudentID, state.BlackTime, NotificationBlacklisted))
		message := strings.TrimSpace(state.BlackMessage)
		if message == "" && strings.TrimSpace(state.BlackTime) != "" {
			message = "图书馆预约权限已暂停至 " + strings.TrimSpace(state.BlackTime) + "。"
		}
		payload := notificationPayload{NotificationType: NotificationBlacklisted, TargetAt: now.Unix(), Message: message}
		return s.enqueuePayload(ctx, sub, key, NotificationBlacklisted, payload)
	}
	return nil
}

// 仅高频率扫描正在使用中的座位
// 减少性能开销
func (s *ReminderService) ScanActive(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	rows, err := s.dao.ActiveSubscriptions(ctx, s.now(), 100000)
	if err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.ActiveReservations.Set(float64(len(rows)))
	}
	return s.forEachSubscription(ctx, rows, s.scanActiveUser)
}

func (s *ReminderService) scanActiveUser(ctx context.Context, sub dao.LibraryReminderSubscription) (err error) {
	currentSub, err := s.dao.Subscription(ctx, sub.StudentID)
	if err != nil || !currentSub.Enabled || currentSub.PreferenceVersion != sub.PreferenceVersion {
		return err
	}
	finish, started := s.userTaskGate.start(sub.StudentID, "active_scan", sub.PreferenceVersion, s.now(), s.config.ActiveScanMinInterval)
	if !started {
		return nil
	}
	defer func() { finish(err == nil) }()
	token, err := s.libraryToken(ctx, sub.StudentID)
	if err != nil {
		return err
	}
	if current, err := s.subscriptionStillCurrent(ctx, sub); err != nil || !current {
		return err
	}
	current, err := s.crawler.GetCurrentReservation(ctx, token)
	if err != nil {
		return err
	}
	now := s.now()
	return s.dao.Transaction(ctx, func(txDAO *dao.ReminderDAO) error {
		locked, err := txDAO.SubscriptionForUpdate(ctx, sub.StudentID)
		if err != nil {
			return err
		}
		if !locked.Enabled || locked.PreferenceVersion != sub.PreferenceVersion {
			return nil
		}
		txService := *s
		txService.dao = txDAO
		return txService.applyActiveObservation(ctx, *locked, current, now)
	})
}

func (s *ReminderService) applyActiveObservation(ctx context.Context, sub dao.LibraryReminderSubscription, current *crawler.ReminderReservation, now time.Time) error {
	if current == nil {
		if episode, findErr := s.dao.LatestActiveAwayEpisode(ctx, sub.StudentID); findErr == nil {
			episode.State = dao.AwayStateEnded
			if err := s.dao.SaveAwayEpisode(ctx, episode); err != nil {
				return err
			}
			if err := s.dao.CancelReservationJobTypes(ctx, sub.StudentID, episode.ExternalReservationID, []string{NotificationAway60, NotificationAway80}); err != nil {
				return err
			}
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		return s.dao.MarkActiveScan(ctx, sub.StudentID, now)
	}
	if oldEpisode, findErr := s.dao.LatestActiveAwayEpisode(ctx, sub.StudentID); findErr == nil && oldEpisode.ExternalReservationID != current.ID {
		oldEpisode.State = dao.AwayStateEnded
		if err := s.dao.SaveAwayEpisode(ctx, oldEpisode); err != nil {
			return err
		}
		if err := s.dao.CancelReservationJobTypes(ctx, sub.StudentID, oldEpisode.ExternalReservationID, []string{NotificationAway60, NotificationAway80, NotificationEnd10}); err != nil {
			return err
		}
	} else if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return findErr
	}
	if _, err := s.dao.Reservation(ctx, sub.StudentID, current.ID); errors.Is(err, gorm.ErrRecordNotFound) {
		if recErr := s.reconcileReservation(ctx, sub, *current, now); recErr != nil {
			return recErr
		}
	} else if err != nil {
		return err
	}
	away := currentAwayObservation(*current, now)
	episode, err := s.dao.LatestAwayEpisode(ctx, sub.StudentID, current.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if !away.isAway {
		if err == nil && episode.State == dao.AwayStateAway {
			episode.State = dao.AwayStateReturned
			if err := s.dao.SaveAwayEpisode(ctx, episode); err != nil {
				return err
			}
			if err := s.dao.CancelReservationJobTypes(ctx, sub.StudentID, current.ID, []string{NotificationAway60, NotificationAway80}); err != nil {
				return err
			}
		}
		return s.dao.MarkActiveScan(ctx, sub.StudentID, now)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		episode = nil
	}
	episode = nextAwayEpisode(episode, sub.StudentID, current.ID, away.elapsedMinutes, now)
	version := episode.EpisodeVersion
	if err := s.dao.SaveAwayEpisode(ctx, episode); err != nil {
		return err
	}
	if s.config.NotificationTypes.Away60 && !episode.Alert60Sent {
		if err := s.scheduleJob(ctx, sub, NotificationAway60, current.ID, version, episode.AwayStartedAt.Add(60*time.Minute), int64(version)); err != nil {
			return err
		}
	}
	if s.config.NotificationTypes.Away80 && !episode.Alert80Sent {
		if err := s.scheduleJob(ctx, sub, NotificationAway80, current.ID, version, episode.AwayStartedAt.Add(80*time.Minute), int64(version)); err != nil {
			return err
		}
	}
	return s.dao.MarkActiveScan(ctx, sub.StudentID, now)
}

func nextAwayEpisode(existing *dao.AwayEpisode, studentID, reservationID string, awayMinutes int, observedAt time.Time) *dao.AwayEpisode {
	if existing != nil && existing.State == dao.AwayStateAway {
		existing.LastAwayMinutes = awayMinutes
		return existing
	}
	version := 1
	if existing != nil {
		version = existing.EpisodeVersion + 1
	}
	return &dao.AwayEpisode{
		StudentID:             studentID,
		ExternalReservationID: reservationID,
		EpisodeVersion:        version,
		AwayStartedAt:         observedAt.Add(-time.Duration(awayMinutes) * time.Minute),
		LastAwayMinutes:       awayMinutes,
		State:                 dao.AwayStateAway,
	}
}

func currentAwayObservation(row crawler.ReminderReservation, observedAt time.Time) awayObservation {
	if row.AwayTimeM > 0 {
		return awayObservation{isAway: true, elapsedMinutes: row.AwayTimeM, elapsedKnown: true}
	}
	if strings.TrimSpace(row.AwayRange) == "" {
		return awayObservation{}
	}
	minutes, ok := awayMinutesFromRange(row.AwayRange, row.MakeDateStr, observedAt)
	if ok {
		return awayObservation{isAway: true, elapsedMinutes: minutes, elapsedKnown: true}
	}
	return awayObservation{isAway: true}
}

var awayRangeClockRE = regexp.MustCompile(`(\d{1,2}):(\d{2})`)

func awayMinutesFromRange(awayRange, makeDate string, observedAt time.Time) (int, bool) {
	match := awayRangeClockRE.FindStringSubmatch(awayRange)
	if match == nil {
		return 0, false
	}
	hour, err := strconv.Atoi(match[1])
	if err != nil || hour < 0 || hour > 23 {
		return 0, false
	}
	minute, err := strconv.Atoi(match[2])
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = observedAt.Location()
	}
	observed := observedAt.In(loc)
	day := time.Date(observed.Year(), observed.Month(), observed.Day(), 0, 0, 0, 0, loc)
	if strings.TrimSpace(makeDate) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", makeDate, loc)
		if err != nil {
			return 0, false
		}
		day = parsed
	}
	startedAt := day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	if startedAt.After(observed) && startedAt.Sub(observed) > 12*time.Hour {
		startedAt = startedAt.Add(-24 * time.Hour)
	}
	if startedAt.After(observed) {
		return 0, false
	}
	return int(observed.Sub(startedAt) / time.Minute), true
}

func (s *ReminderService) DispatchJobs(ctx context.Context) (err error) {
	defer s.observeWorkMetrics(ctx)
	if !s.Enabled() {
		return nil
	}
	jobs, err := s.dao.ClaimDueJobs(ctx, s.now(), 100)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := s.dispatchJob(ctx, job); err != nil {
			s.logger.Warn("library reminder job failed", logger.Int64("job_id", job.ID), logger.String("notification_type", job.Type), logger.Error(err))
		}
	}
	return nil
}

func (s *ReminderService) dispatchJob(ctx context.Context, job dao.NotificationJob) error {
	if !s.notificationEnabled(job.Type) {
		return s.dao.FinishJob(ctx, job, dao.JobSuppressed, "notification type disabled", nil)
	}
	sub, err := s.dao.Subscription(ctx, job.StudentID)
	if err != nil {
		return s.retryJob(ctx, job, err)
	}
	if !sub.Enabled || sub.PreferenceVersion != job.PreferenceVersion {
		return s.dao.FinishJob(ctx, job, dao.JobSuppressed, "subscription changed", nil)
	}
	if job.Type == NotificationAway60 || job.Type == NotificationAway80 {
		threshold := 60
		if job.Type == NotificationAway80 {
			threshold = 80
		}
		token, err := s.libraryToken(ctx, job.StudentID)
		if err != nil {
			return s.retryJob(ctx, job, err)
		}
		// 检查当前时间窗口内，订阅情况是否改变
		if current, err := s.subscriptionStillCurrent(ctx, *sub); err != nil || !current {
			return s.dao.FinishJob(ctx, job, dao.JobSuppressed, "subscription changed", nil)
		}
		current, err := s.crawler.GetCurrentReservation(ctx, token)
		if err != nil {
			return s.retryJob(ctx, job, err)
		}
		now := s.now()
		away := awayObservation{}
		if current != nil {
			away = currentAwayObservation(*current, now)
		}
		if current == nil || current.ID != job.ExternalReservationID || !away.isAway {
			return s.dao.FinishJob(ctx, job, dao.JobSuppressed, "away condition no longer holds", nil)
		}
		if away.elapsedKnown && away.elapsedMinutes < threshold {
			next := now.Add(time.Duration(threshold-away.elapsedMinutes) * time.Minute)
			return s.dao.FinishJob(ctx, job, dao.JobPending, "", &next)
		}
	}
	var start, end time.Time
	var seatID, seatLabel, location string
	if job.ExternalReservationID != "" {
		reservation, findErr := s.dao.Reservation(ctx, job.StudentID, job.ExternalReservationID)
		if findErr != nil {
			return s.retryJob(ctx, job, findErr)
		}
		start, end, seatID, seatLabel, location = reservation.StartAt, reservation.EndAt, reservation.SeatID, reservation.SeatLabel, reservation.Location
	}
	payload := notificationPayload{NotificationType: job.Type, ReservationID: job.ExternalReservationID, SeatID: seatID, SeatLabel: seatLabel, Location: location, StartAt: start.Unix(), EndAt: end.Unix(), TargetAt: job.RunAt.Unix(), EpisodeVersion: job.EpisodeVersion}
	raw, err := json.Marshal(payload)
	if err != nil {
		return s.retryJob(ctx, job, err)
	}
	outbox := &dao.NotificationOutbox{DedupeKey: job.LogicalKey, StudentID: job.StudentID, ExternalReservationID: job.ExternalReservationID, PreferenceVersion: job.PreferenceVersion, Type: job.Type, Payload: raw, Status: dao.OutboxPending, NextAttemptAt: s.now()}
	return s.dao.CompleteJobAndEnqueue(ctx, job, outbox)
}

func (s *ReminderService) retryJob(ctx context.Context, job dao.NotificationJob, cause error) error {
	if job.Attempts >= s.config.RetryMaxAttempts {
		return s.dao.FinishJob(ctx, job, dao.JobSuppressed, "upstream state remained unknown", nil)
	}
	delay := time.Duration(1<<min(job.Attempts, 6)) * time.Minute
	next := s.now().Add(delay)
	if err := s.dao.FinishJob(ctx, job, dao.JobPending, "transient failure", &next); err != nil {
		return err
	}
	return cause
}

func (s *ReminderService) SendOutbox(ctx context.Context) (err error) {
	defer s.observeWorkMetrics(ctx)
	if !s.Enabled() {
		return nil
	}
	rows, err := s.dao.ClaimOutbox(ctx, s.now(), 100, s.config.RetryMaxAttempts)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if !s.notificationEnabled(row.Type) {
			if err := s.dao.FinishOutbox(ctx, row, dao.OutboxSuppressed, "notification type disabled", nil); err != nil {
				return err
			}
			continue
		}
		if s.config.IsDryRun() {
			if err := s.dao.FinishOutbox(ctx, row, dao.OutboxSuppressed, "dry_run", nil); err != nil {
				return err
			}
			continue
		}
		var payload notificationPayload
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			if finishErr := s.dao.FinishOutbox(ctx, row, dao.OutboxSuppressed, "invalid payload", nil); finishErr != nil {
				return finishErr
			}
			continue
		}
		canSend, err := s.dao.CanSendOutbox(ctx, row)
		if err != nil {
			return err
		}
		if !canSend {
			continue
		}
		event := payloadFeedEvent(row.DedupeKey, payload)
		callCtx, cancel := s.remoteCallContext(ctx)
		defer cancel()
		result, publishErr := s.feed.Publish(callCtx, row.StudentID, event)
		if publishErr == nil {
			if result == PublishDuplicate && s.metrics != nil {
				s.metrics.NotificationDeduplicatedTotal.WithLabelValues(row.Type).Inc()
			}
			status := dao.OutboxSent
			if result == PublishSuppressed {
				status = dao.OutboxSuppressed
			}
			if err := s.dao.FinishOutbox(ctx, row, status, "", nil); err != nil {
				return err
			}
			continue
		}
		s.logger.Warn("library reminder outbox publish failed",
			logger.Int64("outbox_id", row.ID),
			logger.String("notification_type", row.Type),
			logger.String("dedupe_key", row.DedupeKey),
			logger.String("student_id", maskStudentID(row.StudentID)),
			logger.Int("attempt", row.Attempts),
			logger.Error(publishErr),
		)
		if row.Attempts >= s.config.RetryMaxAttempts {
			if err := s.dao.FinishOutbox(ctx, row, dao.OutboxFailed, "retry limit reached", nil); err != nil {
				return err
			}
			continue
		}
		next := s.now().Add(time.Duration(1<<min(row.Attempts, 8)) * time.Second)
		if err := s.dao.FinishOutbox(ctx, row, dao.OutboxFailed, "transient publish failure", &next); err != nil {
			return err
		}
	}
	return nil
}

func (s *ReminderService) observeWorkMetrics(ctx context.Context) {
	if s.metrics == nil || s.dao == nil || ctx.Err() != nil {
		return
	}
	snapshot, err := s.dao.MetricsSnapshot(ctx, s.now())
	if err != nil {
		s.logger.Warn("collect library reminder work metrics failed", logger.Error(err))
		return
	}
	s.metrics.NotificationJobs.Reset()
	for _, row := range snapshot.Jobs {
		s.metrics.NotificationJobs.WithLabelValues(row.Type, row.Status).Set(float64(row.Count))
	}
	s.metrics.Outbox.Reset()
	for _, row := range snapshot.Outbox {
		s.metrics.Outbox.WithLabelValues(row.Status).Set(float64(row.Count))
	}
	jobLag := 0.0
	if snapshot.OldestDueJobAt != nil {
		jobLag = max(0, s.now().Sub(*snapshot.OldestDueJobAt).Seconds())
	}
	s.metrics.NotificationJobLagSeconds.Set(jobLag)
	outboxAge := 0.0
	if snapshot.OldestOutboxAt != nil {
		outboxAge = max(0, s.now().Sub(*snapshot.OldestOutboxAt).Seconds())
	}
	s.metrics.OutboxOldestAgeSeconds.Set(outboxAge)
	s.metrics.ActiveReservations.Set(float64(snapshot.ActiveUsers))
}

func (s *ReminderService) scheduleJob(ctx context.Context, sub dao.LibraryReminderSubscription, notificationType, reservationID string, episode int, runAt time.Time, businessVersion int64) error {
	key := reminderJobKey(sub.StudentID, reservationID, notificationType, businessVersion)
	return s.dao.UpsertJob(ctx, &dao.NotificationJob{LogicalKey: key, StudentID: sub.StudentID, ExternalReservationID: reservationID, EpisodeVersion: episode, PreferenceVersion: sub.PreferenceVersion, Type: notificationType, RunAt: runAt, Status: dao.JobPending, Version: 1})
}

func reminderJobKey(studentID, reservationID, notificationType string, businessVersion int64) string {
	return boundedDedupeKey(fmt.Sprintf("%s:%s:%s:%d", studentID, reservationID, notificationType, businessVersion))
}

func (s *ReminderService) notificationEnabled(notificationType string) bool {
	types := s.config.NotificationTypes
	if types == nil {
		return false
	}
	switch notificationType {
	case NotificationReservationDiscovered:
		return types.ReservationDiscovered
	case NotificationStart30:
		return types.Start30
	case NotificationEnd10:
		return types.End10
	case NotificationAway60:
		return types.Away60
	case NotificationAway80:
		return types.Away80
	case NotificationBreach:
		return types.Breach
	case NotificationBlacklisted:
		return types.Blacklisted
	default:
		return false
	}
}

func (s *ReminderService) enqueueFact(ctx context.Context, sub dao.LibraryReminderSubscription, notificationType, reservationID string, episode int, start, end, target time.Time) error {
	key := boundedDedupeKey(fmt.Sprintf("%s:%s:%s", sub.StudentID, reservationID, notificationType))
	reservation, _ := s.dao.Reservation(ctx, sub.StudentID, reservationID)
	payload := notificationPayload{NotificationType: notificationType, ReservationID: reservationID, StartAt: start.Unix(), EndAt: end.Unix(), TargetAt: target.Unix(), EpisodeVersion: episode}
	if reservation != nil {
		payload.SeatID, payload.SeatLabel, payload.Location = reservation.SeatID, reservation.SeatLabel, reservation.Location
	}
	return s.enqueuePayload(ctx, sub, key, notificationType, payload)
}

func (s *ReminderService) enqueuePayload(ctx context.Context, sub dao.LibraryReminderSubscription, key, notificationType string, payload notificationPayload) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.dao.EnqueueOutbox(ctx, &dao.NotificationOutbox{DedupeKey: key, StudentID: sub.StudentID, ExternalReservationID: payload.ReservationID, PreferenceVersion: sub.PreferenceVersion, Type: notificationType, Payload: raw, Status: dao.OutboxPending, NextAttemptAt: s.now()})
}

func (s *ReminderService) libraryToken(ctx context.Context, studentID string) (string, error) {
	callCtx, cancel := s.remoteCallContext(ctx)
	defer cancel()
	resp, err := s.user.GetLibrarySeatToken(callCtx, &userv1.GetLibraryTokenRequest{StudentId: studentID})
	if err != nil {
		return "", err
	}
	if resp == nil || resp.GetToken() == "" {
		return "", errors.New("user service returned an empty library token")
	}
	return resp.GetToken(), nil
}

func (s *ReminderService) remoteCallContext(parent context.Context) (context.Context, context.CancelFunc) {
	if s.config.RequestTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, s.config.RequestTimeout)
}

func (s *ReminderService) subscriptionStillCurrent(ctx context.Context, expected dao.LibraryReminderSubscription) (bool, error) {
	current, err := s.dao.Subscription(ctx, expected.StudentID)
	if err != nil {
		return false, err
	}
	return current.Enabled && current.PreferenceVersion == expected.PreferenceVersion, nil
}

func (s *ReminderService) forEachSubscription(ctx context.Context, rows []dao.LibraryReminderSubscription, fn func(context.Context, dao.LibraryReminderSubscription) error) error {
	limit := s.config.UserConcurrency
	if limit <= 0 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	errCh := make(chan error, len(rows))
	var wg sync.WaitGroup
	var launchErr error

	launching := true
	for i := range rows {
		if !launching {
			break
		}
		if err := ctx.Err(); err != nil {
			launchErr = err
			break
		}
		row := rows[i]
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			launchErr = ctx.Err()
			launching = false
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.runUserOperation(ctx, row, fn); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	first := launchErr
	for err := range errCh {
		if first == nil {
			first = err
		}
	}
	return first
}

func (s *ReminderService) runUserOperation(ctx context.Context, row dao.LibraryReminderSubscription, fn func(context.Context, dao.LibraryReminderSubscription) error) error {
	attempts := s.config.UpstreamRetryAttempts
	if attempts <= 0 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		delay := time.Duration(0)
		if attempt > 0 {
			delay = time.Duration(1<<min(attempt-1, 5)) * 250 * time.Millisecond
		}
		if s.config.UserJitter > 0 {
			delay += time.Duration(rand.Int63n(int64(s.config.UserJitter) + 1))
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		err := fn(ctx, row)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == attempts-1 {
			return err
		}
	}
	return nil
}

func terminalReservationStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "CANCEL", "STOP", "FINISH":
		return true
	default:
		return false
	}
}

func payloadFeedEvent(dedupeKey string, payload notificationPayload) *feedv1.FeedEvent {
	title, content := "图书馆提醒", "你的图书馆预约状态有更新。"
	switch payload.NotificationType {
	case NotificationReservationDiscovered:
		content = fmt.Sprintf("检测到你预约了%s %s。", payload.Location, payload.SeatLabel)
	case NotificationStart30:
		content = fmt.Sprintf("你预约的座位 %s 即将开始。", payload.SeatLabel)
	case NotificationEnd10:
		content = fmt.Sprintf("你预约的座位 %s 即将结束。", payload.SeatLabel)
	case NotificationAway60:
		content = "你已暂离约 60 分钟，请及时返回。"
	case NotificationAway80:
		content = "你已暂离约 80 分钟，请尽快返回。"
	case NotificationBreach:
		content = "检测到新的图书馆违约记录，请查看图书馆规则。"
	case NotificationBlacklisted:
		content = truncateUTF8(payload.Message, 8192)
		if content == "" {
			content = "检测到图书馆预约权限受限，请查看图书馆提示。"
		}
	}
	extend := map[string]string{"notification_type": payload.NotificationType}
	set := func(key, value string) {
		if value != "" {
			extend[key] = value
		}
	}
	set("reservation_id", payload.ReservationID)
	set("seat_id", payload.SeatID)
	set("seat_label", payload.SeatLabel)
	set("location", payload.Location)
	if payload.StartAt != 0 {
		extend["start_at"] = strconvFormat(payload.StartAt)
	}
	if payload.EndAt != 0 {
		extend["end_at"] = strconvFormat(payload.EndAt)
	}
	if payload.TargetAt != 0 {
		extend["target_at"] = strconvFormat(payload.TargetAt)
	}
	if payload.EpisodeVersion != 0 {
		extend["episode_version"] = fmt.Sprint(payload.EpisodeVersion)
	}
	return &feedv1.FeedEvent{Type: feedv1.FeedEventType_LIBRARY, Title: title, Content: content, ExtendFields: extend, DedupeKey: dedupeKey, Source: "library", OccurredAt: payload.TargetAt}
}

func strconvFormat(value int64) string { return fmt.Sprintf("%d", value) }

func boundedDedupeKey(value string) string {
	if len(value) <= 255 {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return "library:sha256:" + hex.EncodeToString(sum[:])
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func maskStudentID(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}
