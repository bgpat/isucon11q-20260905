package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestGraphCacheHitAndClear(t *testing.T) {
	invalidateGraphCache()
	t.Cleanup(invalidateGraphCache)

	key := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "isu", startUnix: 1628521200}
	if _, ok := graphCacheGet(key); ok {
		t.Fatal("expected miss")
	}

	graphCacheSet(key, []byte("body\n"))
	got, ok := graphCacheGet(key)
	if !ok || string(got) != "body\n" {
		t.Fatalf("got %q ok=%v", got, ok)
	}

	invalidateGraphCache()
	if _, ok := graphCacheGet(key); ok {
		t.Fatal("expected miss after clear")
	}
}

func TestGraphCacheInvalidateWindow(t *testing.T) {
	invalidateGraphCache()
	t.Cleanup(invalidateGraphCache)

	start := time.Unix(1628521200, 0)
	key := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "isu", startUnix: start.Unix()}
	other := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "other", startUnix: start.Unix()}
	otherUser := graphCacheKey{jiaUserID: "v", jiaIsuUUID: "isu", startUnix: start.Unix()}
	graphCacheSet(key, []byte("a"))
	graphCacheSet(other, []byte("b"))
	graphCacheSet(otherUser, []byte("c"))

	invalidateGraphCacheForConditions("isu", []time.Time{start.Add(-time.Second)})
	if _, ok := graphCacheGet(key); !ok {
		t.Fatal("timestamp before window should keep cache")
	}

	invalidateGraphCacheForConditions("isu", []time.Time{start})
	if _, ok := graphCacheGet(key); ok {
		t.Fatal("timestamp at start should invalidate")
	}
	if _, ok := graphCacheGet(other); !ok {
		t.Fatal("other uuid should remain")
	}
	if _, ok := graphCacheGet(otherUser); ok {
		t.Fatal("same uuid other user in the window should invalidate")
	}

	graphCacheSet(key, []byte("a"))
	invalidateGraphCacheForConditions("isu", []time.Time{start.Add(24 * time.Hour)})
	if _, ok := graphCacheGet(key); !ok {
		t.Fatal("timestamp at end should keep cache")
	}

	invalidateGraphCacheForConditions("isu", []time.Time{start.Add(24*time.Hour - time.Second)})
	if _, ok := graphCacheGet(key); ok {
		t.Fatal("timestamp last second of window should invalidate")
	}
}

func TestGraphCacheInvalidateOverlappingHours(t *testing.T) {
	invalidateGraphCache()
	t.Cleanup(invalidateGraphCache)

	start := time.Unix(1628521200, 0)
	cur := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "isu", startUnix: start.Unix()}
	prev := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "isu", startUnix: start.Add(-time.Hour).Unix()}
	next := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "isu", startUnix: start.Add(time.Hour).Unix()}
	other := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "keep", startUnix: start.Unix()}
	graphCacheSet(cur, []byte("cur"))
	graphCacheSet(prev, []byte("prev"))
	graphCacheSet(next, []byte("next"))
	graphCacheSet(other, []byte("keep"))
	for i := 0; i < 32; i++ {
		graphCacheSet(graphCacheKey{jiaUserID: "u", jiaIsuUUID: "noise", startUnix: start.Unix() + int64(i)*3600}, []byte("n"))
	}

	invalidateGraphCacheForConditions("isu", []time.Time{start})
	if _, ok := graphCacheGet(cur); ok {
		t.Fatal("start should invalidate current hour window")
	}
	if _, ok := graphCacheGet(prev); ok {
		t.Fatal("start should invalidate previous hour window that still covers start")
	}
	if _, ok := graphCacheGet(next); !ok {
		t.Fatal("next hour window starting after timestamp should remain")
	}
	if _, ok := graphCacheGet(other); !ok {
		t.Fatal("other uuid should remain")
	}

	graphCacheSet(cur, []byte("cur"))
	graphCacheSet(prev, []byte("prev"))
	invalidateGraphCacheForConditions("isu", []time.Time{start.Add(-time.Second)})
	if _, ok := graphCacheGet(cur); !ok {
		t.Fatal("timestamp before current window should keep current hour")
	}
	if _, ok := graphCacheGet(prev); ok {
		t.Fatal("timestamp one second before start should invalidate previous hour")
	}
}

func TestGraphCacheSetIfGenDiscardsStale(t *testing.T) {
	invalidateGraphCache()
	t.Cleanup(invalidateGraphCache)

	key := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "isu", startUnix: 1628521200}
	_, seen, ok := graphCacheLookup(key)
	if ok {
		t.Fatal("expected miss")
	}
	invalidateGraphCacheForConditions("isu", []time.Time{time.Unix(1628521200, 0)})
	if graphCacheSetIfGen(key, []byte("stale\n"), seen) {
		t.Fatal("stale generate should not be stored")
	}
	if _, ok := graphCacheGet(key); ok {
		t.Fatal("cache should stay empty after discarded set")
	}

	_, seen, ok = graphCacheLookup(key)
	if ok {
		t.Fatal("expected miss before fresh set")
	}
	if !graphCacheSetIfGen(key, []byte("fresh\n"), seen) {
		t.Fatal("matching gen should store")
	}
	got, ok := graphCacheGet(key)
	if !ok || string(got) != "fresh\n" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestMarshalPrettyJSONMatchesWriteJSON(t *testing.T) {
	res := []GraphResponse{
		{StartAt: 0, EndAt: 3600, Data: nil, ConditionTimestamps: []int64{}},
		{StartAt: 3600, EndAt: 7200, Data: &GraphDataPoint{Score: 66, Percentage: ConditionsPercentage{Sitting: 50}}, ConditionTimestamps: []int64{3600, 3610}},
	}
	body, err := marshalPrettyJSON(res)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := writeJSON(e.NewContext(req, rec), http.StatusOK, res); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatalf("writeJSON body mismatch\nwriteJSON: %s\nmarshal: %s", rec.Body.Bytes(), body)
	}

	rec2 := httptest.NewRecorder()
	if err := writeJSONBytes(e.NewContext(req, rec2), http.StatusOK, body); err != nil {
		t.Fatal(err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status=%d", rec2.Code)
	}
	if !bytes.Equal(rec2.Body.Bytes(), body) {
		t.Fatalf("writeJSONBytes body mismatch")
	}
}
