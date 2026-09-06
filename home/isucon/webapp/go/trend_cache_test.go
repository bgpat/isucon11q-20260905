package main

import (
	"testing"
	"time"
)

func seedTrendCache(res []TrendResponse) {
	trendCache.Lock()
	trendCache.response = res
	trendCache.valid = true
	trendCache.Unlock()
	noteTrendCacheRecomputed()
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
