package main

import (
	"sync"
	"sync/atomic"
	"time"
)

const trendRecomputeMinInterval = 200 * time.Millisecond

var (
	trendComputeMu  sync.Mutex
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

func loadTrendCache() ([]TrendResponse, bool) {
	trendCache.RLock()
	defer trendCache.RUnlock()
	if !trendCache.valid {
		return nil, false
	}
	return trendCache.response, true
}

func snapshotTrendCache() (gen uint64, res []TrendResponse, hit bool) {
	trendCache.RLock()
	defer trendCache.RUnlock()
	if trendCache.valid {
		return trendCache.gen, trendCache.response, true
	}
	return trendCache.gen, nil, false
}

func storeTrendCache(gen uint64, res []TrendResponse) {
	trendCache.Lock()
	defer trendCache.Unlock()
	if trendCache.gen != gen {
		return
	}
	trendCache.response = res
	trendCache.valid = true
	noteTrendCacheRecomputed()
}

func invalidateTrendCache() {
	trendCache.Lock()
	trendCache.valid = false
	trendCache.response = nil
	trendCache.gen++
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
