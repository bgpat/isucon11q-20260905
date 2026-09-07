package main

import (
	"testing"
	"time"
)

func TestTrendCacheStaleStoreIsIgnored(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)

	gen, _, hit := snapshotTrendCache()
	if hit {
		t.Fatal("expected miss after invalidate")
	}
	storeTrendCache(gen, []TrendResponse{{Character: "ok"}})
	got, ok := loadTrendCache()
	if !ok || len(got) != 1 || got[0].Character != "ok" {
		t.Fatalf("got=%v ok=%v", got, ok)
	}

	invalidateTrendCache()
	storeTrendCache(gen, []TrendResponse{{Character: "stale"}})
	if _, ok := loadTrendCache(); ok {
		t.Fatal("stale gen should not restore cache")
	}

	gen, _, hit = snapshotTrendCache()
	if hit {
		t.Fatal("expected miss after stale store")
	}
	storeTrendCache(gen, []TrendResponse{{Character: "fresh"}})
	got, ok = loadTrendCache()
	if !ok || got[0].Character != "fresh" {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
}

func TestInvalidateTrendCacheDoesNotNeedComputeMu(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(func() {
		trendComputeMu.Unlock()
		invalidateTrendCache()
	})

	trendComputeMu.Lock()
	done := make(chan struct{})
	go func() {
		invalidateTrendCache()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("invalidateTrendCache blocked while trendComputeMu is held")
	}
}
