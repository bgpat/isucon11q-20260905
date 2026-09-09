package main

import "time"

type graphCacheKey struct {
	jiaUserID  string
	jiaIsuUUID string
	startUnix  int64
}

func graphCacheCurrentGen() uint64 {
	graphCache.Lock()
	g := graphCache.gen
	graphCache.Unlock()
	return g
}

func graphCacheGet(key graphCacheKey) ([]byte, bool) {
	body, _, ok := graphCacheLookup(key)
	return body, ok
}

func graphCacheLookup(key graphCacheKey) ([]byte, uint64, bool) {
	graphCache.Lock()
	defer graphCache.Unlock()
	gen := graphCache.gen
	byStart, ok := graphCache.entries[key.jiaIsuUUID]
	if !ok {
		return nil, gen, false
	}
	byUser, ok := byStart[key.startUnix]
	if !ok {
		return nil, gen, false
	}
	body, ok := byUser[key.jiaUserID]
	return body, gen, ok
}

func graphCacheSet(key graphCacheKey, body []byte) {
	stored := make([]byte, len(body))
	copy(stored, body)
	graphCache.Lock()
	defer graphCache.Unlock()
	graphCacheStoreLocked(key, stored)
}

// graphCacheSetIfGen は seenGen のままだったときだけ書く。
// invalidate 後の古い生成結果を cache に残さない。
func graphCacheSetIfGen(key graphCacheKey, body []byte, seenGen uint64) bool {
	stored := make([]byte, len(body))
	copy(stored, body)
	graphCache.Lock()
	defer graphCache.Unlock()
	if graphCache.gen != seenGen {
		return false
	}
	graphCacheStoreLocked(key, stored)
	return true
}

func graphCacheStoreLocked(key graphCacheKey, stored []byte) {
	if graphCache.entries == nil {
		graphCache.entries = make(map[string]map[int64]map[string][]byte)
	}
	byStart := graphCache.entries[key.jiaIsuUUID]
	if byStart == nil {
		byStart = make(map[int64]map[string][]byte)
		graphCache.entries[key.jiaIsuUUID] = byStart
	}
	byUser := byStart[key.startUnix]
	if byUser == nil {
		byUser = make(map[string][]byte)
		byStart[key.startUnix] = byUser
	}
	byUser[key.jiaUserID] = stored
}

func invalidateGraphCache() {
	graphCache.Lock()
	graphCache.entries = nil
	graphCache.gen++
	gen := graphCache.gen
	graphCache.Unlock()
	graphTraceInvalidateAll(gen)
}

func invalidateGraphCacheForConditions(jiaIsuUUID string, timestamps []time.Time) {
	if len(timestamps) == 0 {
		return
	}
	graphCache.Lock()
	byStart, ok := graphCache.entries[jiaIsuUUID]
	deletedWatchDay := false
	if ok && len(byStart) > 0 {
		// startUnix は getIsuGraph で hour truncate 済み。timestamp ts が
		// [start, start+24h) に入る start は hour 境界で (ts-24h, ts] の最大 24 個。
		_, hadWatchDay := byStart[graphTraceWatchDay]
		seenHour := make(map[int64]struct{}, 8)
		for _, ts := range timestamps {
			hour := ts.Truncate(time.Hour).Unix()
			if _, dup := seenHour[hour]; dup {
				continue
			}
			seenHour[hour] = struct{}{}
			end := time.Unix(hour, 0)
			for i := 0; i < 24; i++ {
				startUnix := end.Add(-time.Duration(i) * time.Hour).Unix()
				delete(byStart, startUnix)
				if hadWatchDay && startUnix == graphTraceWatchDay {
					deletedWatchDay = true
				}
			}
		}
		if len(byStart) == 0 {
			delete(graphCache.entries, jiaIsuUUID)
		}
	}
	// cache が空でも世代を進める。生成中の GET が POST 後に古い JSON を書かないため。
	graphCache.gen++
	gen := graphCache.gen
	graphCache.Unlock()
	graphTraceInvalidate(jiaIsuUUID, timestamps, deletedWatchDay, gen)
}
