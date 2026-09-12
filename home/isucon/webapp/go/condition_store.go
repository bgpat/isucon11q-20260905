package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/valkey-io/valkey-go"
)

const (
	conditionZKeyPrefix      = "cond:z:"
	conditionLatestKeyPrefix = "cond:latest:"
	conditionKeyScanMatch    = "cond:*"
	conditionListFetchFactor = 8
	conditionListFetchMin    = 64
	conditionMemberVer1      = 1
	conditionMemberHdrLen    = 18
	flagSitting              = 1 << 0
	flagDirty                = 1 << 1
	flagOverweight           = 1 << 2
	flagBroken               = 1 << 3
	valkeyLatestUpdateLua    = `
local cur = redis.call('GET', KEYS[1])
local newts = tonumber(ARGV[1])
if not cur then
  redis.call('SET', KEYS[1], ARGV[2])
  return 1
end
local sep = string.char(30)
local oldts = tonumber(string.match(cur, '^(%d+)' .. sep))
if not oldts or newts >= oldts then
  redis.call('SET', KEYS[1], ARGV[2])
  return 1
end
return 0
`
)

var (
	valkeyClient       valkey.Client
	conditionUniqSeq   uint64
	packedConditionCSV = [8]string{
		0: "is_dirty=false,is_overweight=false,is_broken=false",
		1: "is_dirty=true,is_overweight=false,is_broken=false",
		2: "is_dirty=false,is_overweight=true,is_broken=false",
		3: "is_dirty=true,is_overweight=true,is_broken=false",
		4: "is_dirty=false,is_overweight=false,is_broken=true",
		5: "is_dirty=true,is_overweight=false,is_broken=true",
		6: "is_dirty=false,is_overweight=true,is_broken=true",
		7: "is_dirty=true,is_overweight=true,is_broken=true",
	}
)

type conditionRecord struct {
	Timestamp time.Time
	IsSitting bool
	Condition string
	Message   string
}

func defaultValkeyAddr() string {
	if addr := os.Getenv("VALKEY_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

func initValkeyClient() error {
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{defaultValkeyAddr()},
		DisableCache: true,
	})
	if err != nil {
		return err
	}
	valkeyClient = wrapValkeyClient(client)
	return nil
}

func conditionZKeyForLevel(jiaIsuUUID, level string) string {
	return conditionZKeyPrefix + level + ":" + jiaIsuUUID
}

func conditionLatestKey(jiaIsuUUID string) string {
	return conditionLatestKeyPrefix + jiaIsuUUID
}

func encodeConditionMember(rec conditionRecord) (string, error) {
	dirty, overweight, broken, ok := parseConditionFlags(rec.Condition)
	if !ok {
		return "", fmt.Errorf("invalid condition")
	}
	uniq := atomic.AddUint64(&conditionUniqSeq, 1)
	buf := make([]byte, conditionMemberHdrLen+len(rec.Message))
	buf[0] = conditionMemberVer1
	var flags byte
	if rec.IsSitting {
		flags |= flagSitting
	}
	if dirty {
		flags |= flagDirty
	}
	if overweight {
		flags |= flagOverweight
	}
	if broken {
		flags |= flagBroken
	}
	buf[1] = flags
	putBEUint64(buf[2:10], uniq)
	putBEUint64(buf[10:18], uint64(rec.Timestamp.Unix()))
	copy(buf[18:], rec.Message)
	return string(buf), nil
}

func decodeConditionMember(member string) (conditionRecord, error) {
	if len(member) < conditionMemberHdrLen || member[0] != conditionMemberVer1 {
		return conditionRecord{}, fmt.Errorf("invalid condition member")
	}
	flags := member[1]
	return conditionRecord{
		Timestamp: time.Unix(int64(beUint64(member[10:18])), 0),
		IsSitting: flags&flagSitting != 0,
		Condition: packedConditionCSV[(flags>>1)&7],
		Message:   member[18:],
	}, nil
}

func putBEUint64(b []byte, v uint64) {
	_ = b[7]
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
}

func beUint64(s string) uint64 {
	_ = s[7]
	return uint64(s[0])<<56 | uint64(s[1])<<48 | uint64(s[2])<<40 | uint64(s[3])<<32 |
		uint64(s[4])<<24 | uint64(s[5])<<16 | uint64(s[6])<<8 | uint64(s[7])
}

func encodeConditionLatest(rec conditionRecord) string {
	return fmt.Sprintf("%d\x1e%t\x1e%s\x1e%s", rec.Timestamp.Unix(), rec.IsSitting, rec.Condition, rec.Message)
}

func decodeConditionLatest(raw string) (conditionRecord, error) {
	parts := strings.SplitN(raw, "\x1e", 4)
	if len(parts) != 4 {
		return conditionRecord{}, fmt.Errorf("invalid condition latest")
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return conditionRecord{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	sitting, err := strconv.ParseBool(parts[1])
	if err != nil {
		return conditionRecord{}, fmt.Errorf("invalid is_sitting: %w", err)
	}
	return conditionRecord{
		Timestamp: time.Unix(ts, 0),
		IsSitting: sitting,
		Condition: parts[2],
		Message:   parts[3],
	}, nil
}

func insertIsuConditionsToValkey(ctx context.Context, jiaIsuUUID string, records []conditionRecord) error {
	if len(records) == 0 {
		return nil
	}
	cmds := make(valkey.Commands, 0, len(records)+1)
	var latest *conditionRecord
	for i := range records {
		rec := records[i]
		member, err := encodeConditionMember(rec)
		if err != nil {
			return err
		}
		level, err := calculateConditionLevel(rec.Condition)
		if err != nil {
			return err
		}
		cmds = append(cmds, valkeyClient.B().Zadd().Key(conditionZKeyForLevel(jiaIsuUUID, level)).ScoreMember().ScoreMember(float64(rec.Timestamp.Unix()), member).Build())
		if latest == nil || !rec.Timestamp.Before(latest.Timestamp) {
			cp := rec
			latest = &cp
		}
	}
	cmds = append(cmds, valkeyClient.B().Eval().Script(valkeyLatestUpdateLua).Numkeys(1).Key(conditionLatestKey(jiaIsuUUID)).Arg(strconv.FormatInt(latest.Timestamp.Unix(), 10)).Arg(encodeConditionLatest(*latest)).Build())
	for _, resp := range valkeyClient.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return err
		}
	}
	return nil
}

func clearConditionKeys(ctx context.Context) error {
	var cursor uint64
	for {
		entry, err := valkeyClient.Do(ctx, valkeyClient.B().Scan().Cursor(cursor).Match(conditionKeyScanMatch).Count(500).Build()).AsScanEntry()
		if err != nil {
			return err
		}
		if len(entry.Elements) > 0 {
			del := make(valkey.Commands, 0, len(entry.Elements))
			for _, key := range entry.Elements {
				del = append(del, valkeyClient.B().Del().Key(key).Build())
			}
			for _, resp := range valkeyClient.DoMulti(ctx, del...) {
				if err := resp.Error(); err != nil {
					return err
				}
			}
		}
		cursor = entry.Cursor
		if cursor == 0 {
			return nil
		}
	}
}

func loadIsuConditionsFromDBToValkey(ctx context.Context, q sqlx.Queryer) error {
	if err := clearConditionKeys(ctx); err != nil {
		return err
	}
	rows, err := q.Queryx("SELECT `jia_isu_uuid`, `timestamp`, `is_sitting`, `condition`, `message` FROM `isu_condition` ORDER BY `jia_isu_uuid`, `timestamp` ASC, `id` ASC")
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		JIAIsuUUID string    `db:"jia_isu_uuid"`
		Timestamp  time.Time `db:"timestamp"`
		IsSitting  bool      `db:"is_sitting"`
		Condition  string    `db:"condition"`
		Message    string    `db:"message"`
	}
	batchByUUID := map[string][]conditionRecord{}
	var currentUUID string
	flushUUID := func(uuid string) error {
		recs := batchByUUID[uuid]
		if len(recs) == 0 {
			return nil
		}
		if err := insertIsuConditionsToValkey(ctx, uuid, recs); err != nil {
			return err
		}
		delete(batchByUUID, uuid)
		return nil
	}
	for rows.Next() {
		var r row
		if err := rows.StructScan(&r); err != nil {
			return err
		}
		if currentUUID != "" && currentUUID != r.JIAIsuUUID {
			if err := flushUUID(currentUUID); err != nil {
				return err
			}
		}
		currentUUID = r.JIAIsuUUID
		batchByUUID[r.JIAIsuUUID] = append(batchByUUID[r.JIAIsuUUID], conditionRecord{
			Timestamp: r.Timestamp,
			IsSitting: r.IsSitting,
			Condition: r.Condition,
			Message:   r.Message,
		})
		if len(batchByUUID[r.JIAIsuUUID]) >= 200 {
			if err := flushUUID(r.JIAIsuUUID); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if currentUUID != "" {
		return flushUUID(currentUUID)
	}
	return nil
}

func getLatestConditionFromValkey(ctx context.Context, jiaIsuUUID string) (*conditionRecord, error) {
	raw, err := valkeyClient.Do(ctx, valkeyClient.B().Get().Key(conditionLatestKey(jiaIsuUUID)).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec, err := decodeConditionLatest(raw)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func getLatestConditionsFromValkey(ctx context.Context, uuids []string) (map[string]conditionRecord, error) {
	out := make(map[string]conditionRecord, len(uuids))
	span := spanFromContext(ctx)
	setSpanInt(span, "valkey.key_count", len(uuids))
	if len(uuids) == 0 {
		setSpanInt(span, "isu.latest_hit", 0)
		return out, nil
	}
	keys := make([]string, len(uuids))
	for i, uuid := range uuids {
		keys[i] = conditionLatestKey(uuid)
	}
	cmd := valkeyClient.B().Mget().Key(keys...).Build()
	arr, err := valkeyClient.Do(ctx, cmd).ToArray()
	if err != nil {
		recordSpanError(span, err)
		return nil, err
	}
	for i, elem := range arr {
		raw, err := elem.ToString()
		if valkey.IsValkeyNil(err) {
			continue
		}
		if err != nil {
			recordSpanError(span, err)
			return nil, err
		}
		rec, err := decodeConditionLatest(raw)
		if err != nil {
			recordSpanError(span, err)
			return nil, err
		}
		out[uuids[i]] = rec
	}
	setSpanInt(span, "isu.latest_hit", len(out))
	return out, nil
}

func conditionListFetchLimit(limit int) int {
	n := limit * conditionListFetchFactor
	if n < conditionListFetchMin {
		return conditionListFetchMin
	}
	return n
}

func conditionLevelHasAll(conditionLevel map[string]interface{}) bool {
	_, info := conditionLevel[conditionLevelInfo]
	_, warning := conditionLevel[conditionLevelWarning]
	_, critical := conditionLevel[conditionLevelCritical]
	return info && warning && critical
}

func conditionListFetchCount(limit int, conditionLevel map[string]interface{}) int {
	if conditionLevelHasAll(conditionLevel) {
		if limit < 1 {
			return 1
		}
		return limit
	}
	return conditionListFetchLimit(limit)
}

func conditionLevelsFromMap(conditionLevel map[string]interface{}) []string {
	levels := make([]string, 0, 3)
	for _, lv := range []string{conditionLevelInfo, conditionLevelWarning, conditionLevelCritical} {
		if _, ok := conditionLevel[lv]; ok {
			levels = append(levels, lv)
		}
	}
	return levels
}

type scoredCondition struct {
	member string
	rec    conditionRecord
}

func decodeScoredMembers(members []string) ([]scoredCondition, error) {
	out := make([]scoredCondition, 0, len(members))
	for _, member := range members {
		rec, err := decodeConditionMember(member)
		if err != nil {
			return nil, err
		}
		out = append(out, scoredCondition{member: member, rec: rec})
	}
	return out, nil
}

func zrevBefore(a, b scoredCondition) bool {
	ta := a.rec.Timestamp.Unix()
	tb := b.rec.Timestamp.Unix()
	if ta != tb {
		return ta > tb
	}
	return a.member > b.member
}

func zrangeBefore(a, b scoredCondition) bool {
	ta := a.rec.Timestamp.Unix()
	tb := b.rec.Timestamp.Unix()
	if ta != tb {
		return ta < tb
	}
	return a.member < b.member
}

func mergeScoredConditions(groups [][]scoredCondition, limit int, before func(a, b scoredCondition) bool) []conditionRecord {
	idx := make([]int, len(groups))
	capOut := limit
	if capOut < 1 {
		for _, g := range groups {
			capOut += len(g)
		}
	}
	out := make([]conditionRecord, 0, capOut)
	for {
		if limit > 0 && len(out) >= limit {
			break
		}
		best := -1
		for i, g := range groups {
			if idx[i] >= len(g) {
				continue
			}
			if best < 0 || before(g[idx[i]], groups[best][idx[best]]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		out = append(out, groups[best][idx[best]].rec)
		idx[best]++
	}
	return out
}

func recordsFromScored(group []scoredCondition) []conditionRecord {
	out := make([]conditionRecord, 0, len(group))
	for _, sm := range group {
		out = append(out, sm.rec)
	}
	return out
}

func conditionScoreRangeDesc(endTime time.Time, startTime time.Time) (max string, min string) {
	max = "(" + strconv.FormatInt(endTime.Unix(), 10)
	min = "-inf"
	if !startTime.IsZero() {
		min = strconv.FormatInt(startTime.Unix(), 10)
	}
	return max, min
}

func getConditionsByTimeDesc(ctx context.Context, jiaIsuUUID string, endTime time.Time, startTime time.Time, limit int, levels []string) ([]conditionRecord, error) {
	max, min := conditionScoreRangeDesc(endTime, startTime)
	if limit < 1 {
		limit = 1
	}
	if len(levels) == 0 {
		return nil, nil
	}
	cmds := make(valkey.Commands, 0, len(levels))
	for _, lv := range levels {
		cmds = append(cmds, valkeyClient.B().Zrevrangebyscore().Key(conditionZKeyForLevel(jiaIsuUUID, lv)).Max(max).Min(min).Limit(0, int64(limit)).Build())
	}
	groups := make([][]scoredCondition, 0, len(levels))
	for _, resp := range valkeyClient.DoMulti(ctx, cmds...) {
		members, err := resp.AsStrSlice()
		if err != nil {
			return nil, err
		}
		group, err := decodeScoredMembers(members)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	if len(groups) == 1 {
		return recordsFromScored(groups[0]), nil
	}
	return mergeScoredConditions(groups, limit, zrevBefore), nil
}

func getConditionsByTimeAscRange(ctx context.Context, jiaIsuUUID string, start time.Time, end time.Time) ([]conditionRecord, error) {
	min := strconv.FormatInt(start.Unix(), 10)
	max := "(" + strconv.FormatInt(end.Unix(), 10)
	levels := []string{conditionLevelInfo, conditionLevelWarning, conditionLevelCritical}
	cmds := make(valkey.Commands, 0, len(levels))
	for _, lv := range levels {
		cmds = append(cmds, valkeyClient.B().Zrangebyscore().Key(conditionZKeyForLevel(jiaIsuUUID, lv)).Min(min).Max(max).Build())
	}
	groups := make([][]scoredCondition, 0, len(levels))
	for _, resp := range valkeyClient.DoMulti(ctx, cmds...) {
		members, err := resp.AsStrSlice()
		if err != nil {
			return nil, err
		}
		group, err := decodeScoredMembers(members)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return mergeScoredConditions(groups, 0, zrangeBefore), nil
}
