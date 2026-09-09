package main

import "time"

type graphCacheKey struct {
	jiaUserID  string
	jiaIsuUUID string
	startUnix  int64
}

func graphCacheGet(key graphCacheKey) ([]byte, bool) {
	graphCache.Lock()
	defer graphCache.Unlock()
	byStart, ok := graphCache.entries[key.jiaIsuUUID]
	if !ok {
		return nil, false
	}
	byUser, ok := byStart[key.startUnix]
	if !ok {
		return nil, false
	}
	body, ok := byUser[key.jiaUserID]
	return body, ok
}

func graphCacheSet(key graphCacheKey, body []byte) {
	stored := make([]byte, len(body))
	copy(stored, body)
	graphCache.Lock()
	defer graphCache.Unlock()
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
	graphCache.Unlock()
	graphTraceInvalidateAll()
}

func invalidateGraphCacheForConditions(jiaIsuUUID string, timestamps []time.Time) {
	if len(timestamps) == 0 {
		return
	}
	graphCache.Lock()
	byStart, ok := graphCache.entries[jiaIsuUUID]
	if !ok || len(byStart) == 0 {
		graphCache.Unlock()
		return
	}
	// startUnix は getIsuGraph で hour truncate 済み。timestamp ts が
	// [start, start+24h) に入る start は hour 境界で (ts-24h, ts] の最大 24 個。
	_, hadWatchDay := byStart[graphTraceWatchDay]
	deletedWatchDay := false
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
	graphCache.Unlock()
	graphTraceInvalidate(jiaIsuUUID, timestamps, deletedWatchDay)
}
