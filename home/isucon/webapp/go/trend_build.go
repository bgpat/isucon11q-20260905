package main

import (
	"context"
	"sort"
)

func buildTrendResponseFromDBValkey(ctx context.Context) ([]TrendResponse, error) {
	characterList := []Isu{}
	if err := db.Select(&characterList, "SELECT `character` FROM `isu` WHERE `character` IS NOT NULL GROUP BY `character`"); err != nil {
		return nil, err
	}

	isuRows := []struct {
		ID         int    `db:"id"`
		JIAIsuUUID string `db:"jia_isu_uuid"`
		Character  string `db:"character"`
	}{}
	if err := db.Select(&isuRows, "SELECT `id`, `jia_isu_uuid`, `character` FROM `isu` WHERE `character` IS NOT NULL"); err != nil {
		return nil, err
	}
	uuids := make([]string, 0, len(isuRows))
	for _, row := range isuRows {
		uuids = append(uuids, row.JIAIsuUUID)
	}
	latestByUUID, err := getLatestConditionsFromValkey(ctx, uuids)
	if err != nil {
		return nil, err
	}

	info := map[string][]*TrendCondition{}
	warning := map[string][]*TrendCondition{}
	critical := map[string][]*TrendCondition{}
	for _, character := range characterList {
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
	items := make([]trendItem, 0, len(isuRows))
	for _, row := range isuRows {
		latest, ok := latestByUUID[row.JIAIsuUUID]
		if !ok {
			continue
		}
		items = append(items, trendItem{
			id:        row.ID,
			character: row.Character,
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

	res := make([]TrendResponse, 0, len(characterList))
	for _, character := range characterList {
		res = append(res, TrendResponse{
			Character: character.Character,
			Info:      info[character.Character],
			Warning:   warning[character.Character],
			Critical:  critical[character.Character],
		})
	}
	return res, nil
}
