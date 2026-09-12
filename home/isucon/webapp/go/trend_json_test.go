package main

import (
	"bytes"
	"strconv"
	"testing"
	"time"
)

func TestEncodeTrendJSONMatchesPrettyJSONAndIndexesTimestamps(t *testing.T) {
	res := []TrendResponse{{
		Character: "cute",
		Info: []*TrendCondition{
			{ID: 12, Timestamp: 1622640527},
			{ID: 1, Timestamp: 1622640500},
		},
		Warning:  []*TrendCondition{},
		Critical: []*TrendCondition{{ID: 3, Timestamp: 1622640000}},
	}}
	want := marshalPrettyTrend(res)
	body, locs, err := encodeTrendJSON(res)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("encodeTrendJSON が pretty JSON と一致しない\ngot:\n%s\nwant:\n%s", body, want)
	}
	for _, item := range []*TrendCondition{res[0].Info[0], res[0].Info[1], res[0].Critical[0]} {
		loc, ok := locs[item.ID]
		if !ok {
			t.Fatalf("isu_id=%d の offset が無い", item.ID)
		}
		got := string(body[loc.tsOff : loc.tsOff+loc.tsLen])
		if got != strconv.FormatInt(item.Timestamp, 10) {
			t.Fatalf("isu_id=%d timestamp bytes=%q loc=%+v", item.ID, got, loc)
		}
	}
	if locs[12].level != conditionLevelInfo || locs[1].level != conditionLevelInfo || locs[3].level != conditionLevelCritical {
		t.Fatalf("level map=%v", locs)
	}
	if len(locs) != 3 {
		t.Fatalf("locs=%d want 3: %v", len(locs), locs)
	}
}

func TestApplyTrendLatestQueuesTimestampUntilRefresh(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)
	res := []TrendResponse{{
		Character: "cute",
		Info: []*TrendCondition{
			{ID: 7, Timestamp: 1622640527},
			{ID: 8, Timestamp: 1622640500},
		},
		Warning:  []*TrendCondition{},
		Critical: []*TrendCondition{},
	}}
	gen, _, _ := snapshotTrendCache()
	storeTrendCache(gen, res)
	rememberIsuMeta(isuMeta{ID: 7, JIAIsuUUID: "u7", Character: "cute"})
	rememberIsuMeta(isuMeta{ID: 8, JIAIsuUUID: "u8", Character: "cute"})

	trendCache.Lock()
	bodyBefore := trendCache.body
	trendCache.Unlock()

	applyTrendLatest("u7", conditionRecord{
		Timestamp: time.Unix(1622640599, 0),
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
	})
	applyTrendLatest("u8", conditionRecord{
		Timestamp: time.Unix(1622640588, 0),
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
	})
	if trendDirty.Load() {
		t.Fatal("同じ位置の timestamp 更新は dirty にしない")
	}
	if _, ok := loadTrendJSONIfUsable(); ok {
		t.Fatal("未反映の timestamp がある JSON を返してはいけない")
	}
	trendCache.Lock()
	if len(trendCache.body) == 0 || &trendCache.body[0] != &bodyBefore[0] {
		t.Fatal("pending のあいだは JSON 本体を作り直さない")
	}
	if bytes.Contains(trendCache.body, []byte("1622640599")) || bytes.Contains(trendCache.body, []byte("1622640588")) {
		t.Fatalf("POST 時点で JSON が書き換わっている: %s", trendCache.body)
	}
	if len(trendCache.pendingTs) != 2 {
		t.Fatalf("pending=%v", trendCache.pendingTs)
	}
	trendCache.Unlock()

	body, ok := refreshTrendJSONFromMemory()
	if !ok {
		t.Fatal("GET 公開で timestamp をまとめて書けない")
	}
	if !bytes.Contains(body, []byte("1622640599")) || !bytes.Contains(body, []byte("1622640588")) {
		t.Fatalf("差し替え後の JSON に新 timestamp が無い: %s", body)
	}
	if bytes.Contains(body, []byte("1622640527")) || bytes.Contains(body, []byte("1622640500")) {
		t.Fatalf("旧 timestamp が残っている: %s", body)
	}
	if _, ok := loadTrendJSONIfUsable(); !ok {
		t.Fatal("公開後は JSON を返せる")
	}
}

func TestApplyTrendLatestReordersThenRefreshFromMemory(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)
	res := []TrendResponse{{
		Character: "cute",
		Info: []*TrendCondition{
			{ID: 2, Timestamp: 1622640527},
			{ID: 1, Timestamp: 1622640500},
		},
		Warning:  []*TrendCondition{},
		Critical: []*TrendCondition{},
	}}
	gen, _, _ := snapshotTrendCache()
	storeTrendCache(gen, res)
	rememberIsuMeta(isuMeta{ID: 1, JIAIsuUUID: "u1", Character: "cute"})

	applyTrendLatest("u1", conditionRecord{
		Timestamp: time.Unix(1622641000, 0),
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
	})
	if !trendDirty.Load() {
		t.Fatal("並びが変わる更新は dirty にする")
	}
	if _, ok := loadTrendJSONIfUsable(); ok {
		t.Fatal("並び変更後の旧 JSON を返してはいけない")
	}
	body, ok := refreshTrendJSONFromMemory()
	if !ok {
		t.Fatal("メモリから encode し直せる")
	}
	idx2 := bytes.Index(body, []byte(`"isu_id": 2`))
	idx1 := bytes.Index(body, []byte(`"isu_id": 1`))
	if idx1 < 0 || idx2 < 0 || idx1 > idx2 {
		t.Fatalf("id=1 が先頭になっていない:\n%s", body)
	}
	if !bytes.Contains(body, []byte("1622641000")) {
		t.Fatalf("新 timestamp が無い:\n%s", body)
	}
}

func TestLoadTrendJSONIfUsableRejectsDirtyBody(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)
	res := []TrendResponse{{
		Character: "cute",
		Info: []*TrendCondition{
			{ID: 2, Timestamp: 1622640527},
			{ID: 1, Timestamp: 1622640500},
		},
		Warning:  []*TrendCondition{},
		Critical: []*TrendCondition{},
	}}
	gen, _, _ := snapshotTrendCache()
	storeTrendCache(gen, res)
	markTrendCacheDirty()
	if _, ok := loadTrendJSONIfUsable(); ok {
		t.Fatal("dirty な旧順 JSON を返してはいけない")
	}
	if _, ok := refreshTrendJSONFromMemory(); !ok {
		t.Fatal("メモリから encode し直せる")
	}
	if _, ok := loadTrendJSONIfUsable(); !ok {
		t.Fatal("refresh 後は JSON を返せる")
	}
}

func TestApplyTrendLatestIgnoresOlderTimestampAcrossReorder(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)
	res := []TrendResponse{{
		Character: "cute",
		Info: []*TrendCondition{
			{ID: 2, Timestamp: 1622640527},
			{ID: 1, Timestamp: 1622640500},
		},
		Warning:  []*TrendCondition{},
		Critical: []*TrendCondition{},
	}}
	gen, _, _ := snapshotTrendCache()
	storeTrendCache(gen, res)
	rememberIsuMeta(isuMeta{ID: 2, JIAIsuUUID: "u2", Character: "cute"})
	rememberIsuMeta(isuMeta{ID: 1, JIAIsuUUID: "u1", Character: "cute"})

	applyTrendLatest("u2", conditionRecord{
		Timestamp: time.Unix(1622640400, 0),
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
	})
	applyTrendLatest("u1", conditionRecord{
		Timestamp: time.Unix(1622641000, 0),
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
	})
	body, ok := refreshTrendJSONFromMemory()
	if !ok {
		t.Fatal("並び変更後の JSON を公開できる")
	}
	if bytes.Contains(body, []byte("1622640400")) {
		t.Fatalf("古い timestamp が encode されている:\n%s", body)
	}
	if !bytes.Contains(body, []byte("1622640527")) {
		t.Fatalf("id=2 の新しめの timestamp が消えている:\n%s", body)
	}
	if !bytes.Contains(body, []byte("1622641000")) {
		t.Fatalf("id=1 の新 timestamp が無い:\n%s", body)
	}
}

func TestApplyTrendLatestIgnoresOlderTimestampWhilePending(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)
	res := []TrendResponse{{
		Character: "cute",
		Info:      []*TrendCondition{{ID: 7, Timestamp: 1622640527}},
		Warning:   []*TrendCondition{},
		Critical:  []*TrendCondition{},
	}}
	gen, _, _ := snapshotTrendCache()
	storeTrendCache(gen, res)
	rememberIsuMeta(isuMeta{ID: 7, JIAIsuUUID: "u7", Character: "cute"})

	applyTrendLatest("u7", conditionRecord{
		Timestamp: time.Unix(1622640599, 0),
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
	})
	applyTrendLatest("u7", conditionRecord{
		Timestamp: time.Unix(1622640580, 0),
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
	})
	body, ok := refreshTrendJSONFromMemory()
	if !ok {
		t.Fatal("pending を公開できる")
	}
	if !bytes.Contains(body, []byte("1622640599")) {
		t.Fatalf("新しい timestamp が無い: %s", body)
	}
	if bytes.Contains(body, []byte("1622640580")) {
		t.Fatalf("pending 中の古い timestamp が勝っている: %s", body)
	}
}
