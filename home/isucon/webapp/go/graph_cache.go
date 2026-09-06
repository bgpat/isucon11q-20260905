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
	body, ok := graphCache.entries[key]
	return body, ok
}

func graphCacheSet(key graphCacheKey, body []byte) {
	stored := make([]byte, len(body))
	copy(stored, body)
	graphCache.Lock()
	defer graphCache.Unlock()
	if graphCache.entries == nil {
		graphCache.entries = make(map[graphCacheKey][]byte)
	}
	graphCache.entries[key] = stored
}

func invalidateGraphCache() {
	graphCache.Lock()
	graphCache.entries = nil
	graphCache.Unlock()
}

func invalidateGraphCacheForConditions(jiaIsuUUID string, timestamps []time.Time) {
	if len(timestamps) == 0 {
		return
	}
	graphCache.Lock()
	defer graphCache.Unlock()
	if len(graphCache.entries) == 0 {
		return
	}
	for key := range graphCache.entries {
		if key.jiaIsuUUID != jiaIsuUUID {
			continue
		}
		start := time.Unix(key.startUnix, 0)
		end := start.Add(24 * time.Hour)
		for _, ts := range timestamps {
			if !ts.Before(start) && ts.Before(end) {
				delete(graphCache.entries, key)
				break
			}
		}
	}
}
