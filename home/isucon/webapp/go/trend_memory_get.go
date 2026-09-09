package main

import (
	"context"
	"sort"
	"sync"
)

type trendIsuEntry struct {
	id        int
	character string
}

type trendMemoryIndex struct {
	mu     sync.RWMutex
	latest map[string]conditionRecord
	isus   map[string]trendIsuEntry
	ready  bool
}

var trendMem trendMemoryIndex

func init() {
	trendMem.resetLocked()
}

func (idx *trendMemoryIndex) resetLocked() {
	idx.latest = make(map[string]conditionRecord)
	idx.isus = make(map[string]trendIsuEntry)
	idx.ready = false
}

func resetTrendMemoryIndex() {
	trendMem.mu.Lock()
	trendMem.resetLocked()
	trendMem.mu.Unlock()
}

func markTrendMemoryNotReady() {
	trendMem.mu.Lock()
	trendMem.ready = false
	trendMem.mu.Unlock()
}

func registerTrendMemoryIsu(jiaIsuUUID string, id int, character string) {
	if character == "" {
		return
	}
	trendMem.mu.Lock()
	trendMem.isus[jiaIsuUUID] = trendIsuEntry{id: id, character: character}
	trendMem.mu.Unlock()
}

func applyTrendMemoryConditions(jiaIsuUUID string, records []conditionRecord) {
	if len(records) == 0 {
		return
	}
	var candidate *conditionRecord
	for i := range records {
		rec := records[i]
		if candidate == nil || !rec.Timestamp.Before(candidate.Timestamp) {
			cp := rec
			candidate = &cp
		}
	}
	if candidate == nil {
		return
	}

	trendMem.mu.Lock()
	defer trendMem.mu.Unlock()
	cur, has := trendMem.latest[jiaIsuUUID]
	if has && candidate.Timestamp.Before(cur.Timestamp) {
		return
	}
	trendMem.latest[jiaIsuUUID] = *candidate
}

func warmTrendMemoryIndex(ctx context.Context) error {
	characterList := []Isu{}
	if err := db.Select(&characterList, "SELECT `character` FROM `isu` WHERE `character` IS NOT NULL GROUP BY `character`"); err != nil {
		return err
	}
	isuRows := []struct {
		ID         int    `db:"id"`
		JIAIsuUUID string `db:"jia_isu_uuid"`
		Character  string `db:"character"`
	}{}
	if err := db.Select(&isuRows, "SELECT `id`, `jia_isu_uuid`, `character` FROM `isu` WHERE `character` IS NOT NULL"); err != nil {
		return err
	}
	uuids := make([]string, 0, len(isuRows))
	for _, row := range isuRows {
		uuids = append(uuids, row.JIAIsuUUID)
	}
	latestByUUID, err := getLatestConditionsFromValkey(ctx, uuids)
	if err != nil {
		return err
	}

	trendMem.mu.Lock()
	defer trendMem.mu.Unlock()
	trendMem.resetLocked()
	for _, row := range isuRows {
		trendMem.isus[row.JIAIsuUUID] = trendIsuEntry{id: row.ID, character: row.Character}
		if latest, ok := latestByUUID[row.JIAIsuUUID]; ok {
			trendMem.latest[row.JIAIsuUUID] = latest
		}
	}
	trendMem.ready = true
	return nil
}

func buildTrendResponseFromMemory() ([]TrendResponse, error) {
	trendMem.mu.RLock()
	defer trendMem.mu.RUnlock()
	if !trendMem.ready {
		return nil, nil
	}

	characterList := []Isu{}
	for _, entry := range trendMem.isus {
		characterList = append(characterList, Isu{Character: entry.character})
	}
	sort.Slice(characterList, func(i, j int) bool {
		return characterList[i].Character < characterList[j].Character
	})
	seenChar := map[string]struct{}{}
	uniqueChars := make([]Isu, 0)
	for _, c := range characterList {
		if _, ok := seenChar[c.Character]; ok {
			continue
		}
		seenChar[c.Character] = struct{}{}
		uniqueChars = append(uniqueChars, c)
	}

	info := map[string][]*TrendCondition{}
	warning := map[string][]*TrendCondition{}
	critical := map[string][]*TrendCondition{}
	for _, character := range uniqueChars {
		info[character.Character] = []*TrendCondition{}
		warning[character.Character] = []*TrendCondition{}
		critical[character.Character] = []*TrendCondition{}
	}

	type trendItem struct {
		id        int
		character string
		timestamp int64
		condition string
	}
	items := make([]trendItem, 0, len(trendMem.isus))
	for uuid, entry := range trendMem.isus {
		latest, ok := trendMem.latest[uuid]
		if !ok {
			continue
		}
		items = append(items, trendItem{
			id:        entry.id,
			character: entry.character,
			timestamp: latest.Timestamp.Unix(),
			condition: latest.Condition,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].character != items[j].character {
			return items[i].character < items[j].character
		}
		if items[i].timestamp != items[j].timestamp {
			return items[i].timestamp > items[j].timestamp
		}
		return items[i].id > items[j].id
	})

	for _, row := range items {
		conditionLevel, err := calculateConditionLevel(row.condition)
		if err != nil {
			return nil, err
		}
		trendCondition := &TrendCondition{ID: row.id, Timestamp: row.timestamp}
		switch conditionLevel {
		case conditionLevelInfo:
			info[row.character] = append(info[row.character], trendCondition)
		case conditionLevelWarning:
			warning[row.character] = append(warning[row.character], trendCondition)
		case conditionLevelCritical:
			critical[row.character] = append(critical[row.character], trendCondition)
		}
	}

	res := make([]TrendResponse, 0, len(uniqueChars))
	for _, character := range uniqueChars {
		res = append(res, TrendResponse{
			Character: character.Character,
			Info:      info[character.Character],
			Warning:   warning[character.Character],
			Critical:  critical[character.Character],
		})
	}
	return res, nil
}

func getTrendFromMemory(ctx context.Context) ([]TrendResponse, error) {
	if res, err := buildTrendResponseFromMemory(); err != nil {
		return nil, err
	} else if res != nil {
		return res, nil
	}
	if err := warmTrendMemoryIndex(ctx); err != nil {
		return nil, err
	}
	return buildTrendResponseFromMemory()
}
