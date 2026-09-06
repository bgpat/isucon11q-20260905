package main

import (
	"sync/atomic"
	"time"
)

const trendRecomputeMinInterval = 200 * time.Millisecond

var (
	trendDirty      atomic.Bool
	trendComputedAt atomic.Int64
)

func markTrendCacheDirty() {
	trendDirty.Store(true)
}

func noteTrendCacheRecomputed() {
	trendDirty.Store(false)
	trendComputedAt.Store(time.Now().UnixNano())
}

func invalidateTrendCache() {
	trendCache.Lock()
	trendCache.valid = false
	trendCache.response = nil
	trendCache.Unlock()
	trendDirty.Store(false)
	trendComputedAt.Store(0)
}

func trendCacheUsableLocked() bool {
	if !trendCache.valid {
		return false
	}
	if !trendDirty.Load() {
		return true
	}
	t := trendComputedAt.Load()
	if t == 0 {
		return false
	}
	return time.Now().UnixNano()-t < int64(trendRecomputeMinInterval)
}

func loadTrendCacheIfUsable() ([]TrendResponse, bool) {
	trendCache.RLock()
	defer trendCache.RUnlock()
	if !trendCacheUsableLocked() {
		return nil, false
	}
	return trendCache.response, true
}
