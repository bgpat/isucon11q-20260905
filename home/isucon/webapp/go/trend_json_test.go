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
	want, err := marshalPrettyJSON(res)
	if err != nil {
		t.Fatal(err)
	}
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
	if locs[12].level != conditionLevelInfo || locs[3].level != conditionLevelCritical {
		t.Fatalf("level map=%v", locs)
	}
}

func TestPatchTrendTimestampSameWidth(t *testing.T) {
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

	meta := isuMeta{ID: 7, JIAIsuUUID: "u", Character: "cute"}
	trendCache.Lock()
	changed := upsertTrendLocked(meta, 1622640599, conditionLevelInfo)
	ok := patchTrendTimestampLocked(7, 1622640599, conditionLevelInfo)
	body := append([]byte(nil), trendCache.body...)
	trendCache.Unlock()
	if changed {
		t.Fatal("同じ位置の timestamp 更新は structure 変更にしない")
	}
	if !ok {
		t.Fatal("同桁の timestamp を差し替えられなかった")
	}
	if !bytes.Contains(body, []byte("1622640599")) {
		t.Fatalf("差し替え後の JSON に新 timestamp が無い: %s", body)
	}
	if bytes.Contains(body, []byte("1622640527")) {
		t.Fatalf("旧 timestamp が残っている: %s", body)
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
	trendComputedAt.Store(time.Now().Add(-trendRecomputeMinInterval - time.Millisecond).UnixNano())
	body, ok := refreshTrendJSONFromMemory()
	if !ok {
		t.Fatal("メモリから encode し直せる")
	}
	info := res // 並びは body を見る
	_ = info
	idx2 := bytes.Index(body, []byte(`"isu_id": 2`))
	idx1 := bytes.Index(body, []byte(`"isu_id": 1`))
	if idx1 < 0 || idx2 < 0 || idx1 > idx2 {
		t.Fatalf("id=1 が先頭になっていない:\n%s", body)
	}
	if !bytes.Contains(body, []byte("1622641000")) {
		t.Fatalf("新 timestamp が無い:\n%s", body)
	}
}
