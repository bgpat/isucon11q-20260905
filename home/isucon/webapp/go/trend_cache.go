package main

import "sync"

// 再計算用。invalidateTrendCache は取らない。
var trendComputeMu sync.Mutex

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
}

func invalidateTrendCache() {
	trendCache.Lock()
	trendCache.valid = false
	trendCache.response = nil
	trendCache.gen++
	trendCache.Unlock()
	resetTrendFlight()
}
