package main

import (
	"strconv"
)

type trendJSONLoc struct {
	tsOff int
	tsLen int
	ts    int64
	level string
}

func encodeTrendJSON(res []TrendResponse) ([]byte, map[int]trendJSONLoc, error) {
	n := 0
	for i := range res {
		n += len(res[i].Info) + len(res[i].Warning) + len(res[i].Critical)
	}
	locs := make(map[int]trendJSONLoc, n)
	body := append(appendPrettyTrendListLocs(make([]byte, 0, prettyTrendCap(res)), res, locs), '\n')
	return body, locs, nil
}

func trendItemBefore(ts1 int64, id1 int, ts2 int64, id2 int) bool {
	if ts1 != ts2 {
		return ts1 > ts2
	}
	return id1 > id2
}

func trendSlice(ch *TrendResponse, level string) *[]*TrendCondition {
	switch level {
	case conditionLevelInfo:
		return &ch.Info
	case conditionLevelWarning:
		return &ch.Warning
	case conditionLevelCritical:
		return &ch.Critical
	default:
		return nil
	}
}

func ensureTrendSlices(ch *TrendResponse) {
	if ch.Info == nil {
		ch.Info = []*TrendCondition{}
	}
	if ch.Warning == nil {
		ch.Warning = []*TrendCondition{}
	}
	if ch.Critical == nil {
		ch.Critical = []*TrendCondition{}
	}
}

func findTrendItem(ch *TrendResponse, id int) (level string, idx int, ok bool) {
	for _, level := range []string{conditionLevelInfo, conditionLevelWarning, conditionLevelCritical} {
		s := *trendSlice(ch, level)
		for i, item := range s {
			if item != nil && item.ID == id {
				return level, i, true
			}
		}
	}
	return "", -1, false
}

func trendStillSorted(s []*TrendCondition, idx int, ts int64, id int) bool {
	if idx > 0 {
		prev := s[idx-1]
		if !trendItemBefore(prev.Timestamp, prev.ID, ts, id) {
			return false
		}
	}
	if idx+1 < len(s) {
		next := s[idx+1]
		if !trendItemBefore(ts, id, next.Timestamp, next.ID) {
			return false
		}
	}
	return true
}

func insertTrendItem(s []*TrendCondition, item *TrendCondition) []*TrendCondition {
	i := 0
	for i < len(s) && trendItemBefore(s[i].Timestamp, s[i].ID, item.Timestamp, item.ID) {
		i++
	}
	s = append(s, nil)
	copy(s[i+1:], s[i:])
	s[i] = item
	return s
}

func removeTrendItem(s []*TrendCondition, idx int) []*TrendCondition {
	copy(s[idx:], s[idx+1:])
	s[len(s)-1] = nil
	return s[:len(s)-1]
}

// upsertTrendLocked は trendCache の write lock を持った状態で呼ぶ。
// 並びや level が変わるときは true。
func upsertTrendLocked(meta isuMeta, ts int64, level string) (structureChanged bool) {
	if !trendCache.valid {
		return true
	}
	idx := -1
	for i := range trendCache.response {
		if trendCache.response[i].Character == meta.Character {
			idx = i
			break
		}
	}
	if idx < 0 {
		trendCache.response = append(trendCache.response, TrendResponse{
			Character: meta.Character,
			Info:      []*TrendCondition{},
			Warning:   []*TrendCondition{},
			Critical:  []*TrendCondition{},
		})
		idx = len(trendCache.response) - 1
		s := trendSlice(&trendCache.response[idx], level)
		if s == nil {
			return true
		}
		*s = insertTrendItem(*s, &TrendCondition{ID: meta.ID, Timestamp: ts})
		return true
	}
	ch := &trendCache.response[idx]
	ensureTrendSlices(ch)
	oldLevel, oldIdx, found := findTrendItem(ch, meta.ID)
	if found {
		oldTs := (*trendSlice(ch, oldLevel))[oldIdx].Timestamp
		if ts < oldTs {
			return false
		}
	}
	if found && oldLevel == level {
		s := trendSlice(ch, level)
		if trendStillSorted(*s, oldIdx, ts, meta.ID) {
			(*s)[oldIdx].Timestamp = ts
			return false
		}
		*s = removeTrendItem(*s, oldIdx)
		*s = insertTrendItem(*s, &TrendCondition{ID: meta.ID, Timestamp: ts})
		return true
	}
	if found {
		old := trendSlice(ch, oldLevel)
		*old = removeTrendItem(*old, oldIdx)
	}
	s := trendSlice(ch, level)
	if s == nil {
		return true
	}
	*s = insertTrendItem(*s, &TrendCondition{ID: meta.ID, Timestamp: ts})
	return true
}

func queueTrendTimestampLocked(id int, ts int64, level string) bool {
	loc, ok := trendCache.locs[id]
	if !ok || loc.level != level || loc.tsLen == 0 {
		return false
	}
	if ts <= loc.ts {
		return true
	}
	if prev, ok := trendCache.pendingTs[id]; ok && ts <= prev {
		return true
	}
	digits := strconv.FormatInt(ts, 10)
	if len(digits) != loc.tsLen {
		return false
	}
	if trendCache.pendingTs == nil {
		trendCache.pendingTs = make(map[int]int64, 1)
	}
	trendCache.pendingTs[id] = ts
	return true
}

func patchTrendTimestampBytes(body []byte, id int, ts int64) bool {
	loc, ok := trendCache.locs[id]
	if !ok || loc.tsLen == 0 {
		return false
	}
	if ts < loc.ts {
		return true
	}
	digits := strconv.FormatInt(ts, 10)
	if len(digits) != loc.tsLen {
		return false
	}
	if loc.tsOff < 0 || loc.tsOff+loc.tsLen > len(body) {
		return false
	}
	copy(body[loc.tsOff:loc.tsOff+loc.tsLen], digits)
	loc.ts = ts
	trendCache.locs[id] = loc
	return true
}

func applyPendingTimestampsLocked() bool {
	if len(trendCache.pendingTs) == 0 {
		return true
	}
	if len(trendCache.body) == 0 {
		return false
	}
	body := make([]byte, len(trendCache.body))
	copy(body, trendCache.body)
	for id, ts := range trendCache.pendingTs {
		if !patchTrendTimestampBytes(body, id, ts) {
			return false
		}
	}
	trendCache.body = body
	clearTrendPendingLocked()
	return true
}

func applyTrendLatest(jiaIsuUUID string, rec conditionRecord) {
	meta, ok := isuMetas.get(jiaIsuUUID)
	if !ok || meta.Character == "" {
		markTrendCacheDirty()
		return
	}
	level, err := calculateConditionLevel(rec.Condition)
	if err != nil {
		markTrendCacheDirty()
		return
	}
	ts := rec.Timestamp.Unix()
	trendCache.Lock()
	defer trendCache.Unlock()
	if !trendCache.valid {
		return
	}
	changed := upsertTrendLocked(meta, ts, level)
	if !changed && !trendDirty.Load() && queueTrendTimestampLocked(meta.ID, ts, level) {
		return
	}
	trendDirty.Store(true)
}
