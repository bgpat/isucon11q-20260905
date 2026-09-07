package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func seedTrendCache(res []TrendResponse) {
	gen, _, _ := snapshotTrendCache()
	storeTrendCache(gen, res)
}

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

func TestTrendDirtyKeepsCacheWithinInterval(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)

	seedTrendCache([]TrendResponse{{Character: "a"}})
	markTrendCacheDirty()
	got, ok := loadTrendCacheIfUsable()
	if !ok || len(got) != 1 || got[0].Character != "a" {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
}

func TestTrendDirtyMissesAfterInterval(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)

	seedTrendCache([]TrendResponse{{Character: "a"}})
	markTrendCacheDirty()
	trendComputedAt.Store(time.Now().Add(-trendRecomputeMinInterval - time.Millisecond).UnixNano())
	if _, ok := loadTrendCacheIfUsable(); ok {
		t.Fatal("dirty cache older than interval should miss")
	}
}

func TestInvalidateTrendCacheMissesImmediately(t *testing.T) {
	seedTrendCache([]TrendResponse{{Character: "a"}})
	invalidateTrendCache()
	if _, ok := loadTrendCacheIfUsable(); ok {
		t.Fatal("hard invalidate should miss")
	}
}

func TestLoadTrendCacheReturnsStaleWhenDirtyExpired(t *testing.T) {
	invalidateTrendCache()
	t.Cleanup(invalidateTrendCache)

	seedTrendCache([]TrendResponse{{Character: "stale"}})
	markTrendCacheDirty()
	trendComputedAt.Store(time.Now().Add(-trendRecomputeMinInterval - time.Millisecond).UnixNano())
	if _, ok := loadTrendCacheIfUsable(); ok {
		t.Fatal("expired dirty cache should not be usable")
	}
	got, ok := loadTrendCache()
	if !ok || got[0].Character != "stale" {
		t.Fatalf("stale cache should remain got=%v ok=%v", got, ok)
	}
}

func TestTryScheduleTrendRefreshRunsOnce(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var n atomic.Int32
	run := func() {
		n.Add(1)
		close(started)
		<-release
	}
	tryScheduleTrendRefresh(run)
	tryScheduleTrendRefresh(run)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	if n.Load() != 1 {
		t.Fatalf("started %d want 1", n.Load())
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !trendRefreshPending.Load() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("refresh pending stuck")
}
