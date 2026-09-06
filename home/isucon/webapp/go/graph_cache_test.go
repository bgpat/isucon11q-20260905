package main

import (
	"bytes"
	"fmt"
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

func TestGraphCacheInvalidateSkipsUnrelatedUUIDs(t *testing.T) {
	invalidateGraphCache()
	t.Cleanup(invalidateGraphCache)

	start := time.Unix(1628521200, 0)
	target := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "target", startUnix: start.Unix()}
	graphCacheSet(target, []byte("t"))
	others := make([]graphCacheKey, 200)
	for i := range others {
		others[i] = graphCacheKey{jiaUserID: "u", jiaIsuUUID: fmt.Sprintf("other-%d", i), startUnix: start.Unix()}
		graphCacheSet(others[i], []byte("o"))
	}

	invalidateGraphCacheForConditions("target", []time.Time{start})
	if _, ok := graphCacheGet(target); ok {
		t.Fatal("target should invalidate")
	}
	for _, key := range others {
		if _, ok := graphCacheGet(key); !ok {
			t.Fatalf("%s should remain", key.jiaIsuUUID)
		}
	}
	graphCache.Lock()
	_, hasIndex := graphCache.byUUID["target"]
	graphCache.Unlock()
	if hasIndex {
		t.Fatal("empty uuid index should be removed")
	}
}

func TestInvalidateGraphCacheFromInsertArgs(t *testing.T) {
	invalidateGraphCache()
	t.Cleanup(invalidateGraphCache)

	start := time.Unix(1628521200, 0)
	key := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "isu", startUnix: start.Unix()}
	other := graphCacheKey{jiaUserID: "u", jiaIsuUUID: "keep", startUnix: start.Unix()}
	graphCacheSet(key, []byte("a"))
	graphCacheSet(other, []byte("b"))

	invalidateGraphCacheFromInsertArgs([]interface{}{
		"isu", start.Add(time.Hour), false, "cond", "m",
		"isu", start.Add(2 * time.Hour), true, "cond", "m",
	})
	if _, ok := graphCacheGet(key); ok {
		t.Fatal("insert args in the window should invalidate")
	}
	if _, ok := graphCacheGet(other); !ok {
		t.Fatal("other uuid should remain")
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
