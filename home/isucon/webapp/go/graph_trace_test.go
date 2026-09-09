package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGraphResponseWatchStats(t *testing.T) {
	res := []GraphResponse{
		{StartAt: graphTraceWatchDay, ConditionTimestamps: []int64{graphTraceWatchDay + 1}},
		{StartAt: graphTraceWatchDay + 23*3600, ConditionTimestamps: []int64{graphTraceWatchTS - 10, graphTraceWatchTS}},
	}
	n, lastHourN, has := graphResponseWatchStats(res)
	if n != 3 || lastHourN != 2 || !has {
		t.Fatalf("n=%d lastHourN=%d has=%v", n, lastHourN, has)
	}
}

func TestBytesContainsUnixToken(t *testing.T) {
	body := []byte("      \"condition_timestamps\": [\n        1628690400,\n        1628693950\n      ]")
	if !graphBodyHasWatchTS(body) {
		t.Fatal("expected watch ts")
	}
	if bytesContainsUnix(body, 16286939500) {
		t.Fatal("should not match longer number")
	}
	if graphBodyHasWatchTS([]byte("1628690400")) {
		t.Fatal("hour start is not watch ts")
	}
}

func TestGraphTraceDisabledIsNoop(t *testing.T) {
	graphTraceOn.Store(false)
	graphTraceOut = nil
	graphTraceHit(graphCacheKey{jiaIsuUUID: graphTraceWatchUUID, startUnix: graphTraceWatchDay}, []byte("1628693950"))
	graphTracePost(graphTraceWatchUUID, []time.Time{time.Unix(graphTraceWatchTS, 0)})
}

func TestGraphTraceWritesWatchLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph-trace.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		graphTraceOn.Store(false)
		graphTraceOut = nil
		f.Close()
	})
	graphTraceOut = f
	graphTraceOn.Store(true)

	key := graphCacheKey{jiaUserID: "u", jiaIsuUUID: graphTraceWatchUUID, startUnix: graphTraceWatchDay}
	graphTraceMiss(key, 1)
	graphTraceSet(key, []byte("1628693950"), 1, true)
	graphTraceBumpGen()
	graphTraceSet(key, []byte("[]"), 1, false)

	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "ev=graph_miss") || !strings.Contains(got, "stale=1") || !strings.Contains(got, "stored=0") {
		t.Fatalf("log=%q", got)
	}
}
