package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/asynccnu/ccnubox-be/be-class/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class/pkg/logx"
)

type fakeCache struct {
	data map[string]string
}

func (f fakeCache) Get(_ context.Context, key string) (string, error) {
	val, ok := f.data[key]
	if !ok {
		return "", errors.New("cache miss")
	}
	return val, nil
}

func (f fakeCache) Set(_ context.Context, _ string, _ interface{}, _ time.Duration) error {
	return nil
}

func (f fakeCache) Del(_ context.Context, _ ...string) error {
	return nil
}

func (f fakeCache) SAdd(_ context.Context, _ string, _ ...interface{}) error {
	return nil
}

func (f fakeCache) SMembers(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (f fakeCache) SExpire(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

type fakeFreeClassRoomData struct {
	queryCalled        bool
	crawlerQueryCalled bool
	crawlerHasData     bool
	crawlerHasErr      error
	allRooms           []string
	replaceErr         error
	replaceCalls       int
}

func (f *fakeFreeClassRoomData) AddClassroomOccupancy(context.Context, string, string, ...model.CTWPair) error {
	return nil
}

func (f *fakeFreeClassRoomData) ReplaceCrawledClassroomOccupancy(context.Context, string, string, int, ...model.CTWPair) error {
	f.replaceCalls++
	return f.replaceErr
}

func (f *fakeFreeClassRoomData) HasCrawledClassroomOccupancy(context.Context, string, string, int) (bool, error) {
	return f.crawlerHasData, f.crawlerHasErr
}

func (f *fakeFreeClassRoomData) ClearClassroomOccupancy(context.Context, string, string) error {
	return nil
}

func (f *fakeFreeClassRoomData) GetAllClassroom(context.Context, string) ([]string, error) {
	return f.allRooms, nil
}

func (f *fakeFreeClassRoomData) RefreshClassroomOccupancy(context.Context) error {
	return nil
}

func (f *fakeFreeClassRoomData) QueryAvailableClassrooms(context.Context, string, string, int, int, int, string, []string) (map[string]bool, error) {
	f.queryCalled = true
	return map[string]bool{"n101": true}, nil
}

func (f *fakeFreeClassRoomData) QueryAvailableClassroomsFromCrawler(context.Context, string, string, int, int, int, string, []string) (map[string]bool, error) {
	f.crawlerQueryCalled = true
	return map[string]bool{"n101": true}, nil
}

type mutableFakeCache struct {
	data      map[string]string
	deleted   []string
	setKeys   []string
	deleteErr error
}

func (f *mutableFakeCache) Get(_ context.Context, key string) (string, error) {
	value, ok := f.data[key]
	if !ok {
		return "", errors.New("cache miss")
	}
	return value, nil
}

func (f *mutableFakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	f.setKeys = append(f.setKeys, key)
	f.data[key] = value.(string)
	return nil
}

func (f *mutableFakeCache) Del(_ context.Context, keys ...string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for _, key := range keys {
		f.deleted = append(f.deleted, key)
		delete(f.data, key)
	}
	return nil
}

func (*mutableFakeCache) SAdd(context.Context, string, ...interface{}) error   { return nil }
func (*mutableFakeCache) SMembers(context.Context, string) ([]string, error)   { return nil, nil }
func (*mutableFakeCache) SExpire(context.Context, string, time.Duration) error { return nil }

func TestToSerializableClassroomStatsSortsNaturally(t *testing.T) {
	stats := toSerializableClassroomStats(map[string][]bool{
		"n102":  {true},
		"n101":  {true},
		"1002":  {true},
		"1001":  {true},
		"n1001": {true},
		"n201":  {true},
	})

	got := make([]string, 0, len(stats))
	for _, stat := range stats {
		got = append(got, stat.Classroom)
	}

	want := []string{"1001", "1002", "n101", "n102", "n201", "n1001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected classroom order: got %v, want %v", got, want)
	}
}

func TestHasUniformAvailability(t *testing.T) {
	if !hasUniformAvailability(map[string][]bool{
		"n101": {true, true},
		"n102": {true, true},
	}) {
		t.Fatal("expected all-true stats to be uniform")
	}

	if hasUniformAvailability(map[string][]bool{
		"n101": {true, false},
		"n102": {true, true},
	}) {
		t.Fatal("expected mixed stats not to be uniform")
	}
}

func TestGetFreeClassRoomFromCacheReturnsErrorOnPartialMiss(t *testing.T) {
	cacheKey := FreeClassRoomCacheKeyPrefix + ":2024:2:6:2:1:1"
	f := NewFreeClassroomBiz(nil, nil, nil, nil, fakeCache{
		data: map[string]string{
			cacheKey: `["n101","n102"]`,
		},
	}, nil, logx.Nop())

	_, err := f.GetFreeClassRoomFromCache(context.Background(), "2024", "2", 6, 2, 1, []int{1, 2}, "n1")
	if err == nil {
		t.Fatal("expected partial cache miss to return an error")
	}
}

func TestGetFreeClassRoomFromCacheRejectsPoisonedValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "null", value: `null`},
		{name: "invalid json", value: `{`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := FreeClassRoomCacheKeyPrefix + ":2024:2:6:2:1:1"
			f := NewFreeClassroomBiz(nil, nil, nil, nil, fakeCache{
				data: map[string]string{key: tt.value},
			}, nil, logx.Nop())

			_, err := f.GetFreeClassRoomFromCache(context.Background(), "2024", "2", 6, 2, 1, []int{1}, "n1")
			if err == nil {
				t.Fatal("expected poisoned cache value to return an error")
			}
		})
	}
}

func TestGetFreeClassRoomFromCacheAcceptsValidEmptyArray(t *testing.T) {
	key := FreeClassRoomCacheKeyPrefix + ":2024:2:6:2:1:1"
	f := NewFreeClassroomBiz(nil, nil, nil, nil, fakeCache{
		data: map[string]string{key: `[]`},
	}, nil, logx.Nop())

	got, err := f.GetFreeClassRoomFromCache(context.Background(), "2024", "2", 6, 2, 1, []int{1}, "n1")
	if err != nil {
		t.Fatalf("expected valid empty array to be accepted: %v", err)
	}
	if rooms, ok := got[1]; ok && len(rooms) != 0 {
		t.Fatalf("expected no available classrooms, got %v", rooms)
	}
}

func TestAcademicTermCode(t *testing.T) {
	for _, year := range []string{"2025", "2025-2026"} {
		got, err := academicTermCode(year, "3")
		if err != nil {
			t.Fatalf("academicTermCode(%q): %v", year, err)
		}
		if got != "2025-2026-3" {
			t.Fatalf("unexpected term code: got %q", got)
		}
	}
}

func TestBuildFreeClassroomQuery(t *testing.T) {
	form, err := buildFreeClassroomQuery("2025", "2", 19)
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}
	if got := form.Get("xnxqh"); got != "2025-2026-2" {
		t.Fatalf("unexpected xnxqh: %q", got)
	}
	if got := form.Get("selectZc"); got != "19" {
		t.Fatalf("unexpected selectZc: %q", got)
	}
	if got := form.Get("selectXq"); got != "1,2,3,4,5,6,7" {
		t.Fatalf("unexpected selectXq: %q", got)
	}
	if got := form.Get("selectJc"); got != "0102,0304,0506,0708,0910,1112" {
		t.Fatalf("unexpected selectJc: %q", got)
	}
}

func TestExtractFreeClassroomsFromQuery2(t *testing.T) {
	periods := []map[string]string{
		{"jc1": "1", "jc2": "2"},
		{"jc1": "3", "jc2": "4"},
		{"jc1": "5", "jc2": "6"},
		{"jc1": "7", "jc2": "8"},
		{"jc1": "9", "jc2": "10"},
		{"jc1": "11", "jc2": "12"},
	}
	cells := make([]any, 42)
	cells[0] = `<iconpark-icon name="shangke"></iconpark-icon>`
	row := make([]any, 0, 46)
	row = append(row, "n401")
	row = append(row, cells...)
	row = append(row, "room-id", "(48/10)", "多媒体教室")
	raw, err := json.Marshal([]any{
		periods,
		1,
		7,
		[]string{"一", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日"},
		[]any{row},
		7,
		make([]any, 42),
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	got, err := extractFreeClassroomsFromQuery2(raw)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(got[1][1]) != 0 || len(got[1][2]) != 0 {
		t.Fatalf("occupied 01/02 period must not be returned as free: %#v", got[1])
	}
	if !reflect.DeepEqual(got[1][3], []string{"n401"}) || !reflect.DeepEqual(got[1][4], []string{"n401"}) {
		t.Fatalf("free 03/04 period not parsed correctly: %#v", got[1])
	}
	if !reflect.DeepEqual(got[2][1], []string{"n401"}) {
		t.Fatalf("free day-two period not parsed correctly: %#v", got[2])
	}

	selected := selectFreeClassrooms(got, 2, 1, []int{1, 3}, "n4")
	if len(selected[1]) != 0 || !reflect.DeepEqual(selected[3], []string{"n401"}) {
		t.Fatalf("unexpected filtered result: %#v", selected)
	}
}

func TestExtractFreeClassroomsFromQuery2RejectsPartialWeek(t *testing.T) {
	raw, err := json.Marshal([]any{
		[]map[string]string{{"jc1": "1", "jc2": "2"}},
		1,
		6,
		[]string{"一", "星期一"},
		[]any{[]any{"n401", nil, nil, nil, nil, nil, nil, "room-id", "(48/10)", "多媒体教室"}},
		6,
		make([]any, 6),
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	if _, err := extractFreeClassroomsFromQuery2(raw); err == nil {
		t.Fatal("expected a partial-week response to be rejected")
	}
}

func TestIsFreeClassroomCellNormalizesBlankStrings(t *testing.T) {
	for _, cell := range []json.RawMessage{
		nil,
		json.RawMessage(`null`),
		json.RawMessage(`""`),
		json.RawMessage(`"  "`),
		json.RawMessage(`"&nbsp;"`),
		json.RawMessage(`"&#160;"`),
	} {
		if !isFreeClassroomCell(cell) {
			t.Errorf("expected %q to be treated as free", cell)
		}
	}

	if isFreeClassroomCell(json.RawMessage(`"<iconpark-icon name=\"shangke\"></iconpark-icon>"`)) {
		t.Fatal("expected an occupied HTML cell not to be treated as free")
	}
}

func TestQueryAvailableClassroomFromLocalRejectsUnreadyData(t *testing.T) {
	data := &fakeFreeClassRoomData{}
	f := NewFreeClassroomBiz(nil, data, nil, nil, fakeCache{data: map[string]string{}}, nil, logx.Nop())

	_, err := f.queryAvailableClassroomFromLocal(context.Background(), "2024", "2", 6, 1, []int{1}, "n1", []string{"n101"})
	if err == nil {
		t.Fatal("expected unready local occupancy data to return an error")
	}
	if data.queryCalled {
		t.Fatal("availability query should not run before local data is ready")
	}
}

func TestQueryAvailableClassroomFromLocalUsesCompletionMarker(t *testing.T) {
	data := &fakeFreeClassRoomData{}
	cache := fakeCache{data: map[string]string{
		classroomOccupancyReadyKey("2024", "2"): Finished,
	}}
	f := NewFreeClassroomBiz(nil, data, nil, nil, cache, nil, logx.Nop())

	if _, err := f.queryAvailableClassroomFromLocal(context.Background(), "2024", "2", 6, 1, []int{1}, "n1", []string{"n101"}); err != nil {
		t.Fatalf("expected completed occupancy data to be queryable: %v", err)
	}
	if !data.queryCalled {
		t.Fatal("expected availability query after completion marker")
	}
}

func TestQueryAvailableClassroomFromLocalPrefersCrawlerMirror(t *testing.T) {
	data := &fakeFreeClassRoomData{crawlerHasData: true}
	cache := fakeCache{data: map[string]string{
		crawledClassroomOccupancyReadyKey("2025", "3", 3): Finished,
		classroomOccupancyReadyKey("2025", "3"):           Finished,
	}}
	f := NewFreeClassroomBiz(nil, data, nil, nil, cache, nil, logx.Nop())

	if _, err := f.queryAvailableClassroomFromLocal(context.Background(), "2025", "3", 3, 1, []int{1}, "n1", []string{"n101"}); err != nil {
		t.Fatalf("expected crawler occupancy data to be queryable: %v", err)
	}
	if !data.crawlerQueryCalled || data.queryCalled {
		t.Fatalf("expected crawler mirror only, crawler=%v classlist=%v", data.crawlerQueryCalled, data.queryCalled)
	}
}

func TestQueryAvailableClassroomFromLocalFallsBackWhenCrawlerMirrorIsEmpty(t *testing.T) {
	data := &fakeFreeClassRoomData{}
	crawlerReadyKey := crawledClassroomOccupancyReadyKey("2025", "3", 3)
	cache := &mutableFakeCache{data: map[string]string{
		crawlerReadyKey:                         Finished,
		classroomOccupancyReadyKey("2025", "3"): Finished,
	}}
	f := NewFreeClassroomBiz(nil, data, nil, nil, cache, nil, logx.Nop())

	if _, err := f.queryAvailableClassroomFromLocal(context.Background(), "2025", "3", 3, 1, []int{1}, "n1", []string{"n101"}); err != nil {
		t.Fatalf("expected empty crawler mirror to fall back to classlist data: %v", err)
	}
	if !data.queryCalled || data.crawlerQueryCalled {
		t.Fatalf("expected classlist fallback only, crawler=%v classlist=%v", data.crawlerQueryCalled, data.queryCalled)
	}
	if _, ok := cache.data[crawlerReadyKey]; ok {
		t.Fatal("expected stale crawler readiness marker to be cleared")
	}
}

func TestPersistCrawledClassroomMirrorClearsReadinessBeforeReplacement(t *testing.T) {
	readyKey := crawledClassroomOccupancyReadyKey("2025", "3", 3)
	cache := &mutableFakeCache{data: map[string]string{readyKey: Finished}}
	data := &fakeFreeClassRoomData{
		allRooms:   []string{"n101"},
		replaceErr: errors.New("partial bulk failure"),
	}
	f := NewFreeClassroomBiz(nil, data, nil, nil, cache, nil, logx.Nop())

	err := f.persistCrawledClassroomMirror(context.Background(), "2025", "3", 3, newFreeClassroomSchedule())
	if err == nil {
		t.Fatal("expected crawler mirror replacement failure")
	}
	if _, ok := cache.data[readyKey]; ok {
		t.Fatal("expected readiness marker to remain absent after replacement failure")
	}
	if len(cache.setKeys) != 0 {
		t.Fatalf("readiness must not be marked after replacement failure: %v", cache.setKeys)
	}
}

func TestPersistCrawledClassroomMirrorRejectsEmptyClassroomCatalog(t *testing.T) {
	data := &fakeFreeClassRoomData{}
	f := NewFreeClassroomBiz(nil, data, nil, nil, &mutableFakeCache{data: map[string]string{}}, nil, logx.Nop())

	err := f.persistCrawledClassroomMirror(context.Background(), "2025", "3", 3, newFreeClassroomSchedule())
	if err == nil || data.replaceCalls != 0 {
		t.Fatalf("expected empty classroom catalog to fail before replacement, err=%v calls=%d", err, data.replaceCalls)
	}
}

func TestStartOnceDeduplicatesConcurrentWork(t *testing.T) {
	var inFlight sync.Map
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	calls := 0

	if !startOnce(&inFlight, "2025:3:3", func() {
		calls++
		close(started)
		<-release
		close(finished)
	}, nil) {
		t.Fatal("expected first repair to start")
	}
	<-started
	for i := 0; i < 20; i++ {
		if startOnce(&inFlight, "2025:3:3", func() { calls++ }, nil) {
			t.Fatal("expected duplicate repair to be suppressed")
		}
	}
	if calls != 1 {
		t.Fatalf("expected one in-flight repair, got %d", calls)
	}
	close(release)
	<-finished

	deadline := time.Now().Add(time.Second)
	for {
		if _, loaded := inFlight.Load("2025:3:3"); !loaded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("repair key was not released")
		}
		time.Sleep(time.Millisecond)
	}

	secondFinished := make(chan struct{})
	if !startOnce(&inFlight, "2025:3:3", func() {
		calls++
		close(secondFinished)
	}, nil) {
		t.Fatal("expected a later repair to start after the first completed")
	}
	<-secondFinished
	if calls != 2 {
		t.Fatalf("expected two sequential repairs, got %d", calls)
	}
}

func TestStartOnceRecoversPanicAndReleasesKey(t *testing.T) {
	var inFlight sync.Map
	panicValue := make(chan any, 1)
	if !startOnce(&inFlight, "2025:3:3", func() {
		panic("broken cached schedule")
	}, func(recovered any) {
		panicValue <- recovered
	}) {
		t.Fatal("expected repair to start")
	}

	select {
	case recovered := <-panicValue:
		if recovered != "broken cached schedule" {
			t.Fatalf("unexpected recovered panic: %v", recovered)
		}
	case <-time.After(time.Second):
		t.Fatal("background panic was not recovered")
	}

	deadline := time.Now().Add(time.Second)
	for {
		if _, loaded := inFlight.Load("2025:3:3"); !loaded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("repair key was not released after panic")
		}
		time.Sleep(time.Millisecond)
	}

	completed := make(chan struct{})
	if !startOnce(&inFlight, "2025:3:3", func() { close(completed) }, nil) {
		t.Fatal("expected a later repair to start after panic recovery")
	}
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("later repair did not run")
	}
}

func TestBuildCrawledClassroomOccupancy(t *testing.T) {
	schedule := newFreeClassroomSchedule()
	schedule[1][1] = []string{"n101"}
	schedule[1][2] = []string{"n102"}

	pairs := buildCrawledClassroomOccupancy(schedule, 3, []string{"n101", "n102"})
	occupied := make(map[string]map[int]bool)
	for _, pair := range pairs {
		if pair.CT.Day != 1 || len(pair.CT.Weeks) != 1 || pair.CT.Weeks[0] != 3 {
			continue
		}
		if occupied[pair.Where] == nil {
			occupied[pair.Where] = make(map[int]bool)
		}
		for _, section := range pair.CT.Sections {
			occupied[pair.Where][section] = true
		}
	}
	if occupied["n101"][1] || !occupied["n101"][2] {
		t.Fatalf("unexpected n101 occupancy: %v", occupied["n101"])
	}
	if !occupied["n102"][1] || occupied["n102"][2] {
		t.Fatalf("unexpected n102 occupancy: %v", occupied["n102"])
	}
}

func TestValidateFreeClassroomQuery(t *testing.T) {
	if err := validateFreeClassroomQuery("2024-2025", "2", 6, 1, []int{1, 2}, "n1"); err != nil {
		t.Fatalf("expected valid query: %v", err)
	}

	invalid := []struct {
		name        string
		year        string
		semester    string
		week        int
		day         int
		sections    []int
		wherePrefix string
	}{
		{name: "empty year", semester: "2", week: 6, day: 1, sections: []int{1}, wherePrefix: "n1"},
		{name: "invalid semester", year: "2024", semester: "4", week: 6, day: 1, sections: []int{1}, wherePrefix: "n1"},
		{name: "invalid week", year: "2024", semester: "2", week: 0, day: 1, sections: []int{1}, wherePrefix: "n1"},
		{name: "invalid day", year: "2024", semester: "2", week: 6, day: 8, sections: []int{1}, wherePrefix: "n1"},
		{name: "empty sections", year: "2024", semester: "2", week: 6, day: 1, wherePrefix: "n1"},
		{name: "invalid section", year: "2024", semester: "2", week: 6, day: 1, sections: []int{13}, wherePrefix: "n1"},
		{name: "empty prefix", year: "2024", semester: "2", week: 6, day: 1, sections: []int{1}},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateFreeClassroomQuery(tt.year, tt.semester, tt.week, tt.day, tt.sections, tt.wherePrefix); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
