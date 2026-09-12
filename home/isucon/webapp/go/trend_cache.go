package main

import (
	"sync"
	"sync/atomic"
)

var (
	trendComputeMu sync.Mutex
	trendDirty     atomic.Bool
)

func markTrendCacheDirty() {
	trendDirty.Store(true)
}

func noteTrendCacheRecomputed() {
	trendDirty.Store(false)
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
	body, locs, err := encodeTrendJSON(res)
	if err != nil {
		return
	}
	trendCache.Lock()
	defer trendCache.Unlock()
	if trendCache.gen != gen {
		return
	}
	trendCache.response = res
	trendCache.body = body
	trendCache.locs = locs
	trendCache.valid = true
	noteTrendCacheRecomputed()
}

func invalidateTrendCache() {
	trendCache.Lock()
	trendCache.valid = false
	trendCache.response = nil
	trendCache.body = nil
	trendCache.locs = nil
	trendCache.gen++
	trendCache.Unlock()
	trendDirty.Store(false)
}

func trendCacheUsableLocked() bool {
	return trendCache.valid && len(trendCache.body) > 0 && !trendDirty.Load()
}

func loadTrendCacheIfUsable() ([]TrendResponse, bool) {
	trendCache.RLock()
	defer trendCache.RUnlock()
	if !trendCacheUsableLocked() {
		return nil, false
	}
	return trendCache.response, true
}

func loadTrendJSONIfUsable() ([]byte, bool) {
	trendCache.RLock()
	defer trendCache.RUnlock()
	if !trendCacheUsableLocked() {
		return nil, false
	}
	return trendCache.body, true
}

func refreshTrendJSONFromMemory() ([]byte, bool) {
	trendCache.Lock()
	defer trendCache.Unlock()
	if !trendCache.valid || trendCache.response == nil {
		return nil, false
	}
	body, locs, err := encodeTrendJSON(trendCache.response)
	if err != nil {
		return nil, false
	}
	trendCache.body = body
	trendCache.locs = locs
	noteTrendCacheRecomputed()
	return body, true
}
