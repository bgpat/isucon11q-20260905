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
	if graphCache.byUUID == nil {
		graphCache.byUUID = make(map[string]map[graphCacheKey]struct{})
	}
	graphCache.entries[key] = stored
	index := graphCache.byUUID[key.jiaIsuUUID]
	if index == nil {
		index = make(map[graphCacheKey]struct{})
		graphCache.byUUID[key.jiaIsuUUID] = index
	}
	index[key] = struct{}{}
}

func invalidateGraphCache() {
	graphCache.Lock()
	graphCache.entries = nil
	graphCache.byUUID = nil
	graphCache.Unlock()
}

func invalidateGraphCacheFromInsertArgs(args []interface{}) {
	byUUID := make(map[string][]time.Time)
	for i := 0; i+1 < len(args); i += 5 {
		uuid, okUUID := args[i].(string)
		ts, okTS := args[i+1].(time.Time)
		if !okUUID || !okTS || uuid == "" {
			continue
		}
		byUUID[uuid] = append(byUUID[uuid], ts)
	}
	for uuid, timestamps := range byUUID {
		invalidateGraphCacheForConditions(uuid, timestamps)
	}
}

func invalidateGraphCacheForConditions(jiaIsuUUID string, timestamps []time.Time) {
	if len(timestamps) == 0 {
		return
	}
	graphCache.Lock()
	defer graphCache.Unlock()
	keys := graphCache.byUUID[jiaIsuUUID]
	if len(keys) == 0 {
		return
	}
	for key := range keys {
		start := time.Unix(key.startUnix, 0)
		end := start.Add(24 * time.Hour)
		for _, ts := range timestamps {
			if !ts.Before(start) && ts.Before(end) {
				delete(graphCache.entries, key)
				delete(keys, key)
				break
			}
		}
	}
	if len(keys) == 0 {
		delete(graphCache.byUUID, jiaIsuUUID)
	}
}
