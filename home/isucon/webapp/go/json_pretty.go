package main

import (
	"strconv"
	"sync"
)

const (
	jsonHex              = "0123456789abcdef"
	prettyJSONPoolMaxCap = 1 << 20
)

var prettyJSONPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 8192)
		return &b
	},
}

func getPrettyJSONBuf() *[]byte {
	return prettyJSONPool.Get().(*[]byte)
}

func putPrettyJSONBuf(p *[]byte) {
	if p == nil || cap(*p) > prettyJSONPoolMaxCap {
		return
	}
	*p = (*p)[:0]
	prettyJSONPool.Put(p)
}

func growBytes(dst []byte, n int) []byte {
	if cap(dst) >= n {
		return dst[:0]
	}
	return make([]byte, 0, n)
}

func marshalPrettyGraph(list []GraphResponse) []byte {
	return append(appendPrettyGraphList(make([]byte, 0, prettyGraphCap(list)), list), '\n')
}

func marshalPrettyTrend(list []TrendResponse) []byte {
	return append(appendPrettyTrendList(make([]byte, 0, prettyTrendCap(list)), list), '\n')
}

func prettyConditionCap(x []*GetIsuConditionResponse) int {
	if x == nil {
		return 8
	}
	return 64 + len(x)*384
}

func prettyGraphCap(x []GraphResponse) int {
	if x == nil {
		return 8
	}
	return 64 + len(x)*320
}

func prettyIsuListCap(x []GetIsuListResponse) int {
	if x == nil {
		return 8
	}
	return 64 + len(x)*512
}

func prettyTrendCap(x []TrendResponse) int {
	if x == nil {
		return 8
	}
	return 64 + len(x)*256
}

func appendJSONBool(dst []byte, v bool) []byte {
	if v {
		return append(dst, "true"...)
	}
	return append(dst, "false"...)
}

func appendJSONInt(dst []byte, v int64) []byte {
	return strconv.AppendInt(dst, v, 10)
}

func appendJSONQuoted(dst []byte, s string) []byte {
	dst = append(dst, '"')
	dst = append(dst, s...)
	return append(dst, '"')
}

func appendJSONConditionLevel(dst []byte, s string) []byte {
	switch s {
	case conditionLevelInfo:
		return append(dst, `"info"`...)
	case conditionLevelWarning:
		return append(dst, `"warning"`...)
	case conditionLevelCritical:
		return append(dst, `"critical"`...)
	default:
		return appendJSONString(dst, s)
	}
}

func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c < 0x80 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
			continue
		}
		if c >= 0x80 {
			if c == 0xe2 && i+2 < len(s) && s[i+1] == 0x80 && (s[i+2] == 0xa8 || s[i+2] == 0xa9) {
				if start < i {
					dst = append(dst, s[start:i]...)
				}
				if s[i+2] == 0xa8 {
					dst = append(dst, '\\', 'u', '2', '0', '2', '8')
				} else {
					dst = append(dst, '\\', 'u', '2', '0', '2', '9')
				}
				i += 2
				start = i + 1
			}
			continue
		}
		if start < i {
			dst = append(dst, s[start:i]...)
		}
		switch c {
		case '"', '\\':
			dst = append(dst, '\\', c)
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '<':
			dst = append(dst, '\\', 'u', '0', '0', '3', 'c')
		case '>':
			dst = append(dst, '\\', 'u', '0', '0', '3', 'e')
		case '&':
			dst = append(dst, '\\', 'u', '0', '0', '2', '6')
		default:
			dst = append(dst, '\\', 'u', '0', '0', jsonHex[c>>4], jsonHex[c&0xF])
		}
		start = i + 1
	}
	if start < len(s) {
		dst = append(dst, s[start:]...)
	}
	return append(dst, '"')
}

func appendPrettyConditionList(dst []byte, list []*GetIsuConditionResponse) []byte {
	if list == nil {
		return append(dst, "null"...)
	}
	if len(list) == 0 {
		return append(dst, '[', ']')
	}
	dst = append(dst, '[')
	for i, c := range list {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, "\n  "...)
		if c == nil {
			dst = append(dst, "null"...)
			continue
		}
		dst = appendPrettyConditionObj(dst, c)
	}
	return append(dst, "\n]"...)
}

func appendPrettyConditionObj(dst []byte, c *GetIsuConditionResponse) []byte {
	dst = append(dst, "{\n    \"jia_isu_uuid\": "...)
	dst = appendJSONQuoted(dst, c.JIAIsuUUID)
	dst = append(dst, ",\n    \"isu_name\": "...)
	dst = appendJSONString(dst, c.IsuName)
	dst = append(dst, ",\n    \"timestamp\": "...)
	dst = appendJSONInt(dst, c.Timestamp)
	dst = append(dst, ",\n    \"is_sitting\": "...)
	dst = appendJSONBool(dst, c.IsSitting)
	dst = append(dst, ",\n    \"condition\": "...)
	dst = appendJSONQuoted(dst, c.Condition)
	dst = append(dst, ",\n    \"condition_level\": "...)
	dst = appendJSONConditionLevel(dst, c.ConditionLevel)
	dst = append(dst, ",\n    \"message\": "...)
	dst = appendJSONString(dst, c.Message)
	return append(dst, "\n  }"...)
}

func appendPrettyConditionObjNested(dst []byte, c *GetIsuConditionResponse) []byte {
	dst = append(dst, "{\n      \"jia_isu_uuid\": "...)
	dst = appendJSONQuoted(dst, c.JIAIsuUUID)
	dst = append(dst, ",\n      \"isu_name\": "...)
	dst = appendJSONString(dst, c.IsuName)
	dst = append(dst, ",\n      \"timestamp\": "...)
	dst = appendJSONInt(dst, c.Timestamp)
	dst = append(dst, ",\n      \"is_sitting\": "...)
	dst = appendJSONBool(dst, c.IsSitting)
	dst = append(dst, ",\n      \"condition\": "...)
	dst = appendJSONQuoted(dst, c.Condition)
	dst = append(dst, ",\n      \"condition_level\": "...)
	dst = appendJSONConditionLevel(dst, c.ConditionLevel)
	dst = append(dst, ",\n      \"message\": "...)
	dst = appendJSONString(dst, c.Message)
	return append(dst, "\n    }"...)
}

func appendPrettyGraphList(dst []byte, list []GraphResponse) []byte {
	if list == nil {
		return append(dst, "null"...)
	}
	if len(list) == 0 {
		return append(dst, '[', ']')
	}
	dst = append(dst, '[')
	for i := range list {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, "\n  "...)
		dst = appendPrettyGraphObj(dst, &list[i])
	}
	return append(dst, "\n]"...)
}

func appendPrettyGraphObj(dst []byte, g *GraphResponse) []byte {
	dst = append(dst, "{\n    \"start_at\": "...)
	dst = appendJSONInt(dst, g.StartAt)
	dst = append(dst, ",\n    \"end_at\": "...)
	dst = appendJSONInt(dst, g.EndAt)
	dst = append(dst, ",\n    \"data\": "...)
	if g.Data == nil {
		dst = append(dst, "null"...)
	} else {
		dst = appendPrettyGraphData(dst, g.Data)
	}
	dst = append(dst, ",\n    \"condition_timestamps\": "...)
	dst = appendPrettyInt64Slice4(dst, g.ConditionTimestamps)
	return append(dst, "\n  }"...)
}

func appendPrettyGraphData(dst []byte, d *GraphDataPoint) []byte {
	dst = append(dst, "{\n      \"score\": "...)
	dst = appendJSONInt(dst, int64(d.Score))
	dst = append(dst, ",\n      \"percentage\": {\n        \"sitting\": "...)
	dst = appendJSONInt(dst, int64(d.Percentage.Sitting))
	dst = append(dst, ",\n        \"is_broken\": "...)
	dst = appendJSONInt(dst, int64(d.Percentage.IsBroken))
	dst = append(dst, ",\n        \"is_dirty\": "...)
	dst = appendJSONInt(dst, int64(d.Percentage.IsDirty))
	dst = append(dst, ",\n        \"is_overweight\": "...)
	dst = appendJSONInt(dst, int64(d.Percentage.IsOverweight))
	return append(dst, "\n      }\n    }"...)
}

func appendPrettyInt64Slice4(dst []byte, list []int64) []byte {
	if list == nil {
		return append(dst, "null"...)
	}
	if len(list) == 0 {
		return append(dst, '[', ']')
	}
	dst = append(dst, '[')
	for i, n := range list {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, "\n      "...)
		dst = appendJSONInt(dst, n)
	}
	return append(dst, "\n    ]"...)
}

func appendPrettyIsuList(dst []byte, list []GetIsuListResponse) []byte {
	if list == nil {
		return append(dst, "null"...)
	}
	if len(list) == 0 {
		return append(dst, '[', ']')
	}
	dst = append(dst, '[')
	for i := range list {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, "\n  "...)
		dst = appendPrettyIsuListObj(dst, &list[i])
	}
	return append(dst, "\n]"...)
}

func appendPrettyIsuListObj(dst []byte, item *GetIsuListResponse) []byte {
	dst = append(dst, "{\n    \"id\": "...)
	dst = appendJSONInt(dst, int64(item.ID))
	dst = append(dst, ",\n    \"jia_isu_uuid\": "...)
	dst = appendJSONQuoted(dst, item.JIAIsuUUID)
	dst = append(dst, ",\n    \"name\": "...)
	dst = appendJSONString(dst, item.Name)
	dst = append(dst, ",\n    \"character\": "...)
	dst = appendJSONString(dst, item.Character)
	dst = append(dst, ",\n    \"latest_isu_condition\": "...)
	if item.LatestIsuCondition == nil {
		dst = append(dst, "null"...)
	} else {
		dst = appendPrettyConditionObjNested(dst, item.LatestIsuCondition)
	}
	return append(dst, "\n  }"...)
}

func appendPrettyTrendList(dst []byte, list []TrendResponse) []byte {
	return appendPrettyTrendListLocs(dst, list, nil)
}

func appendPrettyTrendListLocs(dst []byte, list []TrendResponse, locs map[int]trendJSONLoc) []byte {
	if list == nil {
		return append(dst, "null"...)
	}
	if len(list) == 0 {
		return append(dst, '[', ']')
	}
	dst = append(dst, '[')
	for i := range list {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, "\n  "...)
		dst = appendPrettyTrendObj(dst, &list[i], locs)
	}
	return append(dst, "\n]"...)
}

func appendPrettyTrendObj(dst []byte, t *TrendResponse, locs map[int]trendJSONLoc) []byte {
	dst = append(dst, "{\n    \"character\": "...)
	dst = appendJSONString(dst, t.Character)
	dst = append(dst, ",\n    \"info\": "...)
	dst = appendPrettyTrendConditions(dst, t.Info, conditionLevelInfo, locs)
	dst = append(dst, ",\n    \"warning\": "...)
	dst = appendPrettyTrendConditions(dst, t.Warning, conditionLevelWarning, locs)
	dst = append(dst, ",\n    \"critical\": "...)
	dst = appendPrettyTrendConditions(dst, t.Critical, conditionLevelCritical, locs)
	return append(dst, "\n  }"...)
}

func appendPrettyTrendConditions(dst []byte, list []*TrendCondition, level string, locs map[int]trendJSONLoc) []byte {
	if list == nil {
		return append(dst, "null"...)
	}
	if len(list) == 0 {
		return append(dst, '[', ']')
	}
	dst = append(dst, '[')
	for i, c := range list {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, "\n      "...)
		if c == nil {
			dst = append(dst, "null"...)
			continue
		}
		dst = append(dst, "{\n        \"isu_id\": "...)
		dst = appendJSONInt(dst, int64(c.ID))
		dst = append(dst, ",\n        \"timestamp\": "...)
		tsOff := len(dst)
		dst = appendJSONInt(dst, c.Timestamp)
		if locs != nil {
			locs[c.ID] = trendJSONLoc{tsOff: tsOff, tsLen: len(dst) - tsOff, ts: c.Timestamp, level: level}
		}
		dst = append(dst, "\n      }"...)
	}
	return append(dst, "\n    ]"...)
}

func appendPrettyInitialize(dst []byte, v InitializeResponse) []byte {
	dst = append(dst, "{\n  \"language\": "...)
	dst = appendJSONQuoted(dst, v.Language)
	return append(dst, "\n}"...)
}

func appendPrettyMe(dst []byte, v GetMeResponse) []byte {
	dst = append(dst, "{\n  \"jia_user_id\": "...)
	dst = appendJSONString(dst, v.JIAUserID)
	return append(dst, "\n}"...)
}

func appendPrettyIsu(dst []byte, v Isu) []byte {
	dst = append(dst, "{\n  \"id\": "...)
	dst = appendJSONInt(dst, int64(v.ID))
	dst = append(dst, ",\n  \"jia_isu_uuid\": "...)
	dst = appendJSONQuoted(dst, v.JIAIsuUUID)
	dst = append(dst, ",\n  \"name\": "...)
	dst = appendJSONString(dst, v.Name)
	dst = append(dst, ",\n  \"character\": "...)
	dst = appendJSONString(dst, v.Character)
	return append(dst, "\n}"...)
}
