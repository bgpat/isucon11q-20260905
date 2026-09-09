package main

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
)

type trendPlacement struct {
	character string
	level     string
}

type trendCharBuckets struct {
	info     []*TrendCondition
	warning  []*TrendCondition
	critical []*TrendCondition
}

type trendRanking struct {
	mu        sync.Mutex
	byChar    map[string]*trendCharBuckets
	charOrder []string
	placement map[int]trendPlacement
	ready     bool
	snapshot  atomic.Value
}

var trendRank trendRanking

func init() {
	trendRank.resetLocked()
}

func (r *trendRanking) resetLocked() {
	r.byChar = make(map[string]*trendCharBuckets)
	r.charOrder = nil
	r.placement = make(map[int]trendPlacement)
	r.ready = false
	r.snapshot.Store([]TrendResponse(nil))
}

func resetTrendRanking() {
	trendRank.mu.Lock()
	trendRank.resetLocked()
	trendRank.mu.Unlock()
}

func cloneTrendResponse(in []TrendResponse) []TrendResponse {
	out := make([]TrendResponse, len(in))
	for i, tr := range in {
		out[i].Character = tr.Character
		if tr.Info != nil {
			out[i].Info = append([]*TrendCondition(nil), tr.Info...)
		}
		if tr.Warning != nil {
			out[i].Warning = append([]*TrendCondition(nil), tr.Warning...)
		}
		if tr.Critical != nil {
			out[i].Critical = append([]*TrendCondition(nil), tr.Critical...)
		}
	}
	return out
}

func (r *trendRanking) publishLocked() {
	res := make([]TrendResponse, 0, len(r.charOrder))
	for _, char := range r.charOrder {
		b := r.byChar[char]
		if b == nil {
			res = append(res, TrendResponse{Character: char})
			continue
		}
		res = append(res, TrendResponse{
			Character: char,
			Info:      append([]*TrendCondition(nil), b.info...),
			Warning:   append([]*TrendCondition(nil), b.warning...),
			Critical:  append([]*TrendCondition(nil), b.critical...),
		})
	}
	r.snapshot.Store(cloneTrendResponse(res))
}

func (r *trendRanking) ensureCharLocked(character string) *trendCharBuckets {
	b := r.byChar[character]
	if b == nil {
		b = &trendCharBuckets{}
		r.byChar[character] = b
		r.charOrder = append(r.charOrder, character)
	}
	return b
}

func insertTrendConditionSorted(slice []*TrendCondition, item *TrendCondition) []*TrendCondition {
	i := sort.Search(len(slice), func(j int) bool {
		if slice[j].Timestamp != item.Timestamp {
			return slice[j].Timestamp < item.Timestamp
		}
		return slice[j].ID < item.ID
	})
	slice = append(slice, nil)
	copy(slice[i+1:], slice[i:])
	slice[i] = item
	return slice
}

func removeTrendCondition(slice []*TrendCondition, isuID int) []*TrendCondition {
	for i, tc := range slice {
		if tc.ID == isuID {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func (r *trendRanking) patchIsu(isuID int, character string, timestamp int64, condition string) error {
	level, err := calculateConditionLevel(condition)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	b := r.ensureCharLocked(character)
	if old, ok := r.placement[isuID]; ok {
		ob := r.byChar[old.character]
		if ob != nil {
			switch old.level {
			case conditionLevelInfo:
				ob.info = removeTrendCondition(ob.info, isuID)
			case conditionLevelWarning:
				ob.warning = removeTrendCondition(ob.warning, isuID)
			case conditionLevelCritical:
				ob.critical = removeTrendCondition(ob.critical, isuID)
			}
		}
	}

	tc := &TrendCondition{ID: isuID, Timestamp: timestamp}
	switch level {
	case conditionLevelInfo:
		b.info = insertTrendConditionSorted(b.info, tc)
	case conditionLevelWarning:
		b.warning = insertTrendConditionSorted(b.warning, tc)
	case conditionLevelCritical:
		b.critical = insertTrendConditionSorted(b.critical, tc)
	default:
		return nil
	}
	r.placement[isuID] = trendPlacement{character: character, level: level}
	r.ready = true
	r.publishLocked()
	return nil
}

func warmTrendRanking(ctx context.Context) error {
	res, err := buildTrendResponseFromDBValkey(ctx)
	if err != nil {
		return err
	}

	trendRank.mu.Lock()
	defer trendRank.mu.Unlock()
	trendRank.resetLocked()
	for _, tr := range res {
		b := trendRank.ensureCharLocked(tr.Character)
		for _, tc := range tr.Info {
			cp := *tc
			b.info = append(b.info, &cp)
			trendRank.placement[tc.ID] = trendPlacement{character: tr.Character, level: conditionLevelInfo}
		}
		for _, tc := range tr.Warning {
			cp := *tc
			b.warning = append(b.warning, &cp)
			trendRank.placement[tc.ID] = trendPlacement{character: tr.Character, level: conditionLevelWarning}
		}
		for _, tc := range tr.Critical {
			cp := *tc
			b.critical = append(b.critical, &cp)
			trendRank.placement[tc.ID] = trendPlacement{character: tr.Character, level: conditionLevelCritical}
		}
	}
	trendRank.ready = true
	trendRank.publishLocked()
	return nil
}

func loadTrendRankingSnapshot() ([]TrendResponse, bool) {
	v := trendRank.snapshot.Load()
	if v == nil {
		return nil, false
	}
	res, ok := v.([]TrendResponse)
	if !ok || !trendRank.ready {
		return nil, false
	}
	return res, true
}

func getTrendFromRanking(ctx context.Context) ([]TrendResponse, error) {
	if res, ok := loadTrendRankingSnapshot(); ok {
		return res, nil
	}
	if err := warmTrendRanking(ctx); err != nil {
		return nil, err
	}
	res, ok := loadTrendRankingSnapshot()
	if !ok {
		return []TrendResponse{}, nil
	}
	return res, nil
}

func patchTrendRankingFromCondition(jiaIsuUUID string, records []conditionRecord, latestUpdated bool) error {
	if !latestUpdated || len(records) == 0 {
		return nil
	}
	meta, ok, err := loadIsuMetaByUUID(jiaIsuUUID)
	if err != nil {
		return err
	}
	if !ok || meta.Character == "" {
		return nil
	}
	var latest *conditionRecord
	for i := range records {
		rec := records[i]
		if latest == nil || !rec.Timestamp.Before(latest.Timestamp) {
			cp := rec
			latest = &cp
		}
	}
	if latest == nil {
		return nil
	}
	return trendRank.patchIsu(meta.ID, meta.Character, latest.Timestamp.Unix(), latest.Condition)
}

func registerTrendRankingIsu(jiaIsuUUID string, id int, character string) error {
	if character == "" {
		return nil
	}
	meta := isuMeta{ID: id, JIAIsuUUID: jiaIsuUUID, Character: character}
	// activate 直後に condition が既にある場合は warm 相当の patch を試みる。
	latestByUUID, err := getLatestConditionsFromValkey(context.Background(), []string{jiaIsuUUID})
	if err != nil {
		return err
	}
	latest, ok := latestByUUID[jiaIsuUUID]
	if !ok {
		return nil
	}
	return trendRank.patchIsu(meta.ID, meta.Character, latest.Timestamp.Unix(), latest.Condition)
}
