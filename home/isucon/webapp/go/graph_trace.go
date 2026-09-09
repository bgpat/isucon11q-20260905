package main

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// 比較ベンチ用。提出計測では無効のままにする。
// 有効化: 環境変数 ISUCON_GRAPH_TRACE=1、または /tmp/isucon-work/graph-trace.on を置いて再起動。
// 無効化: 両方を外して再起動。ログは Git に入れない。
const (
	graphTraceFlagPath  = "/tmp/isucon-work/graph-trace.on"
	graphTraceLogPath   = "/tmp/isucon-work/graph-trace.log"
	graphTraceWatchTS   = int64(1628693950)
	graphTraceWatchDay  = int64(1628607600)
	graphTraceWatchUUID = "a69fb865-7feb-4864-995d-f88cec319cd5"
)

var (
	graphTraceOn  atomic.Bool
	graphTraceGen atomic.Uint64
	graphTraceMu  sync.Mutex
	graphTraceOut *os.File
)

func initGraphTrace() {
	if getEnv("ISUCON_GRAPH_TRACE", "0") == "1" {
		graphTraceOn.Store(true)
	}
	if _, err := os.Stat(graphTraceFlagPath); err == nil {
		graphTraceOn.Store(true)
	}
	if !graphTraceOn.Load() {
		return
	}
	if err := os.MkdirAll("/tmp/isucon-work", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "graph-trace mkdir: %v\n", err)
	}
	path := getEnv("ISUCON_GRAPH_TRACE_LOG", graphTraceLogPath)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "graph-trace open %s: %v\n", path, err)
		graphTraceOut = os.Stderr
		graphTraceLog("ev=trace_start log=stderr")
		return
	}
	graphTraceOut = f
	graphTraceLog("ev=trace_start log=" + path)
}

func graphTraceEnabled() bool {
	return graphTraceOn.Load()
}

func graphTraceBumpGen() uint64 {
	return graphTraceGen.Add(1)
}

func graphTraceCurrentGen() uint64 {
	return graphTraceGen.Load()
}

func graphTraceLog(msg string) {
	if !graphTraceEnabled() {
		return
	}
	line := fmt.Sprintf("%s %s\n", time.Now().Format("15:04:05.000000"), msg)
	graphTraceMu.Lock()
	defer graphTraceMu.Unlock()
	out := graphTraceOut
	if out == nil {
		out = os.Stderr
	}
	_, _ = out.WriteString(line)
}

func graphTraceWatchDayKey(startUnix int64) bool {
	return startUnix == graphTraceWatchDay
}

func graphTraceWatchUUIDKey(jiaIsuUUID string) bool {
	return jiaIsuUUID == graphTraceWatchUUID
}

func graphBodyHasWatchTS(body []byte) bool {
	// pretty JSON の整数トークン。1628690400 / 1628694000 とは一致しない。
	return bytesContainsUnix(body, graphTraceWatchTS)
}

func bytesContainsUnix(body []byte, ts int64) bool {
	needle := []byte(fmt.Sprintf("%d", ts))
	for i := 0; i+len(needle) <= len(body); i++ {
		if !bytesEqualAt(body, i, needle) {
			continue
		}
		if i > 0 && isASCIIDigit(body[i-1]) {
			continue
		}
		end := i + len(needle)
		if end < len(body) && isASCIIDigit(body[end]) {
			continue
		}
		return true
	}
	return false
}

func bytesEqualAt(body []byte, i int, needle []byte) bool {
	for j := 0; j < len(needle); j++ {
		if body[i+j] != needle[j] {
			return false
		}
	}
	return true
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func graphResponseWatchStats(res []GraphResponse) (n, lastHourN int, hasWatch bool) {
	for _, g := range res {
		n += len(g.ConditionTimestamps)
		if g.StartAt == graphTraceWatchDay+23*3600 {
			lastHourN = len(g.ConditionTimestamps)
		}
		for _, ts := range g.ConditionTimestamps {
			if ts == graphTraceWatchTS {
				hasWatch = true
			}
		}
	}
	return n, lastHourN, hasWatch
}

func graphTraceHit(key graphCacheKey, body []byte) {
	if !graphTraceEnabled() {
		return
	}
	if !graphTraceWatchDayKey(key.startUnix) && !graphTraceWatchUUIDKey(key.jiaIsuUUID) {
		return
	}
	has := 0
	if graphBodyHasWatchTS(body) {
		has = 1
	}
	graphTraceLog(fmt.Sprintf("ev=graph_hit uuid=%s user=%s start=%d has_watch=%d gen=%d",
		key.jiaIsuUUID, key.jiaUserID, key.startUnix, has, graphTraceCurrentGen()))
}

func graphTraceMiss(key graphCacheKey, seenGen uint64) {
	if !graphTraceEnabled() {
		return
	}
	if !graphTraceWatchDayKey(key.startUnix) && !graphTraceWatchUUIDKey(key.jiaIsuUUID) {
		return
	}
	graphTraceLog(fmt.Sprintf("ev=graph_miss uuid=%s user=%s start=%d gen=%d",
		key.jiaIsuUUID, key.jiaUserID, key.startUnix, seenGen))
}

func graphTraceGenerated(key graphCacheKey, res []GraphResponse, seenGen uint64) {
	if !graphTraceEnabled() {
		return
	}
	n, lastHourN, hasWatch := graphResponseWatchStats(res)
	has := 0
	if hasWatch {
		has = 1
	}
	if !graphTraceWatchDayKey(key.startUnix) && !graphTraceWatchUUIDKey(key.jiaIsuUUID) && has == 0 {
		return
	}
	graphTraceLog(fmt.Sprintf("ev=graph_gen uuid=%s user=%s start=%d ts_n=%d last_hour_n=%d has_watch=%d gen=%d",
		key.jiaIsuUUID, key.jiaUserID, key.startUnix, n, lastHourN, has, seenGen))
}

func graphTraceSet(key graphCacheKey, body []byte, seenGen uint64) {
	if !graphTraceEnabled() {
		return
	}
	nowGen := graphTraceCurrentGen()
	stale := 0
	if nowGen != seenGen {
		stale = 1
	}
	has := 0
	if graphBodyHasWatchTS(body) {
		has = 1
	}
	if stale == 0 && !graphTraceWatchDayKey(key.startUnix) && !graphTraceWatchUUIDKey(key.jiaIsuUUID) {
		return
	}
	graphTraceLog(fmt.Sprintf("ev=graph_set uuid=%s user=%s start=%d has_watch=%d gen_seen=%d gen_now=%d stale=%d",
		key.jiaIsuUUID, key.jiaUserID, key.startUnix, has, seenGen, nowGen, stale))
}

func graphTraceInvalidateAll() {
	if !graphTraceEnabled() {
		return
	}
	gen := graphTraceBumpGen()
	graphTraceLog(fmt.Sprintf("ev=graph_inv_all gen=%d", gen))
}

func graphTraceInvalidate(jiaIsuUUID string, timestamps []time.Time, deletedWatchDay bool) {
	if !graphTraceEnabled() {
		return
	}
	gen := graphTraceBumpGen()
	if !deletedWatchDay && !graphTraceWatchUUIDKey(jiaIsuUUID) {
		return
	}
	n := len(timestamps)
	var first, last int64
	inWatchDay := 0
	if n > 0 {
		first = timestamps[0].Unix()
		last = timestamps[n-1].Unix()
		for _, ts := range timestamps {
			u := ts.Unix()
			if u >= graphTraceWatchDay && u < graphTraceWatchDay+24*3600 {
				inWatchDay++
			}
		}
	}
	watch := 0
	if deletedWatchDay {
		watch = 1
	}
	graphTraceLog(fmt.Sprintf("ev=graph_inv uuid=%s nts=%d first=%d last=%d in_watch_day=%d deleted_watch_day=%d gen=%d",
		jiaIsuUUID, n, first, last, inWatchDay, watch, gen))
}

func graphTraceValkeyRange(jiaIsuUUID string, startUnix int64, records []conditionRecord) {
	if !graphTraceEnabled() {
		return
	}
	if !graphTraceWatchDayKey(startUnix) && !graphTraceWatchUUIDKey(jiaIsuUUID) {
		return
	}
	has := 0
	lastHourN := 0
	for _, rec := range records {
		u := rec.Timestamp.Unix()
		if u == graphTraceWatchTS {
			has = 1
		}
		if u >= graphTraceWatchDay+23*3600 && u < graphTraceWatchDay+24*3600 {
			lastHourN++
		}
	}
	graphTraceLog(fmt.Sprintf("ev=graph_valkey uuid=%s start=%d n=%d last_hour_n=%d has_watch=%d",
		jiaIsuUUID, startUnix, len(records), lastHourN, has))
}

func graphTracePost(jiaIsuUUID string, timestamps []time.Time) {
	if !graphTraceEnabled() {
		return
	}
	if !graphTraceWatchUUIDKey(jiaIsuUUID) {
		return
	}
	n := len(timestamps)
	var first, last int64
	if n > 0 {
		first = timestamps[0].Unix()
		last = timestamps[n-1].Unix()
	}
	graphTraceLog(fmt.Sprintf("ev=post uuid=%s nts=%d first=%d last=%d",
		jiaIsuUUID, n, first, last))
}

func graphTraceCond(jiaIsuUUID string, rows []*GetIsuConditionResponse) {
	if !graphTraceEnabled() {
		return
	}
	has := 0
	for _, row := range rows {
		if row != nil && row.Timestamp == graphTraceWatchTS {
			has = 1
			break
		}
	}
	if has == 0 && !graphTraceWatchUUIDKey(jiaIsuUUID) {
		return
	}
	n := len(rows)
	var first, last int64
	if n > 0 && rows[0] != nil {
		first = rows[0].Timestamp
		last = rows[n-1].Timestamp
	}
	graphTraceLog(fmt.Sprintf("ev=cond uuid=%s n=%d first=%d last=%d has_watch=%d",
		jiaIsuUUID, n, first, last, has))
}
