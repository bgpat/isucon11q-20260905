package main

import (
	"strings"
	"testing"
	"time"
)

func TestEncodeDecodeConditionMember(t *testing.T) {
	rec := conditionRecord{
		Timestamp: time.Unix(1700000000, 0),
		IsSitting: true,
		Condition: "is_dirty=false,is_overweight=true,is_broken=false",
		Message:   "hello",
	}
	member, err := encodeConditionMember(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(member, "is_dirty=") || strings.Contains(member, "\x1e") {
		t.Fatalf("member still looks like text: %q", member)
	}
	got, err := decodeConditionMember(member)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSitting != rec.IsSitting || got.Condition != rec.Condition || got.Message != rec.Message {
		t.Fatalf("got=%+v want=%+v", got, rec)
	}
	if got.Timestamp.Unix() != rec.Timestamp.Unix() {
		t.Fatalf("timestamp=%d want %d", got.Timestamp.Unix(), rec.Timestamp.Unix())
	}
	if got.Condition != packedConditionCSV[2] {
		t.Fatalf("condition not interned: %q", got.Condition)
	}
}

func TestEncodeDecodeConditionMemberEmptyMessage(t *testing.T) {
	rec := conditionRecord{
		Timestamp: time.Unix(1, 0),
		IsSitting: false,
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
		Message:   "",
	}
	member, err := encodeConditionMember(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeConditionMember(member)
	if err != nil {
		t.Fatal(err)
	}
	if got != rec {
		t.Fatalf("got=%+v want=%+v", got, rec)
	}
}

func TestEncodeConditionMemberRejectsInvalid(t *testing.T) {
	_, err := encodeConditionMember(conditionRecord{
		Timestamp: time.Unix(1, 0),
		Condition: "bad",
		Message:   "m",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeConditionMemberRejectsShort(t *testing.T) {
	if _, err := decodeConditionMember(""); err == nil {
		t.Fatal("expected error")
	}
	bad := string([]byte{conditionMemberVer1 + 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if _, err := decodeConditionMember(bad); err == nil {
		t.Fatal("expected error")
	}
}

func TestPackedConditionCSVMatchesParser(t *testing.T) {
	for i, csv := range packedConditionCSV {
		dirty, overweight, broken, ok := parseConditionFlags(csv)
		if !ok {
			t.Fatalf("csv[%d]=%q not parseable", i, csv)
		}
		var flags byte
		if dirty {
			flags |= flagDirty
		}
		if overweight {
			flags |= flagOverweight
		}
		if broken {
			flags |= flagBroken
		}
		if int(flags>>1) != i {
			t.Fatalf("csv[%d] packed=%d", i, flags>>1)
		}
		rec := conditionRecord{
			Timestamp: time.Unix(int64(i+1), 0),
			IsSitting: i%2 == 0,
			Condition: csv,
			Message:   "m",
		}
		member, err := encodeConditionMember(rec)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeConditionMember(member)
		if err != nil {
			t.Fatal(err)
		}
		if got.Condition != csv || got.IsSitting != rec.IsSitting || got.Message != rec.Message || got.Timestamp.Unix() != rec.Timestamp.Unix() {
			t.Fatalf("i=%d got=%+v want=%+v", i, got, rec)
		}
	}
}

func TestEncodeDecodeConditionLatest(t *testing.T) {
	rec := conditionRecord{
		Timestamp: time.Unix(1700000001, 0),
		IsSitting: false,
		Condition: "is_dirty=true,is_overweight=true,is_broken=true",
		Message:   "msg",
	}
	raw := encodeConditionLatest(rec)
	got, err := decodeConditionLatest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != rec {
		t.Fatalf("got=%+v want=%+v", got, rec)
	}
}

func TestConditionListFetchLimit(t *testing.T) {
	if got := conditionListFetchLimit(conditionLimit); got != 160 {
		t.Fatalf("limit=%d fetch=%d want 160", conditionLimit, got)
	}
	if got := conditionListFetchLimit(1); got != conditionListFetchMin {
		t.Fatalf("limit=1 fetch=%d want %d", got, conditionListFetchMin)
	}
	if got := conditionListFetchLimit(20); got != 20*conditionListFetchFactor {
		t.Fatalf("limit=20 fetch=%d want %d", got, 20*conditionListFetchFactor)
	}
}

func TestConditionListFetchCountAllLevels(t *testing.T) {
	all := map[string]interface{}{
		conditionLevelInfo:     struct{}{},
		conditionLevelWarning:  struct{}{},
		conditionLevelCritical: struct{}{},
	}
	if got := conditionListFetchCount(conditionLimit, all); got != conditionLimit {
		t.Fatalf("all levels fetch=%d want %d", got, conditionLimit)
	}
	onlyInfo := map[string]interface{}{conditionLevelInfo: struct{}{}}
	if got := conditionListFetchCount(conditionLimit, onlyInfo); got != 160 {
		t.Fatalf("info only fetch=%d want 160", got)
	}
	infoWarning := map[string]interface{}{
		conditionLevelInfo:    struct{}{},
		conditionLevelWarning: struct{}{},
	}
	if got := conditionListFetchCount(conditionLimit, infoWarning); got != 160 {
		t.Fatalf("info+warning fetch=%d want 160", got)
	}
}

func TestDefaultValkeyAddrUsesEnv(t *testing.T) {
	t.Setenv("VALKEY_ADDR", "example:6380")
	if got := defaultValkeyAddr(); got != "example:6380" {
		t.Fatalf("addr=%q", got)
	}
}

func TestConditionMembersAreUnique(t *testing.T) {
	rec := conditionRecord{
		Timestamp: time.Unix(1, 0),
		IsSitting: false,
		Condition: "is_dirty=false,is_overweight=false,is_broken=false",
		Message:   "m",
	}
	a, err := encodeConditionMember(rec)
	if err != nil {
		t.Fatal(err)
	}
	b, err := encodeConditionMember(rec)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("members should differ: %q", a)
	}
}

func TestConditionLevelAllowMask(t *testing.T) {
	all := map[string]interface{}{
		conditionLevelInfo:     struct{}{},
		conditionLevelWarning:  struct{}{},
		conditionLevelCritical: struct{}{},
	}
	if got := conditionLevelAllowMask(all); got != 0xf {
		t.Fatalf("all mask=%d want 15", got)
	}
	if got := conditionLevelAllowMask(map[string]interface{}{conditionLevelInfo: struct{}{}}); got != 1 {
		t.Fatalf("info mask=%d want 1", got)
	}
	if got := conditionLevelAllowMask(map[string]interface{}{conditionLevelWarning: struct{}{}}); got != 6 {
		t.Fatalf("warning mask=%d want 6", got)
	}
	if got := conditionLevelAllowMask(map[string]interface{}{conditionLevelCritical: struct{}{}}); got != 8 {
		t.Fatalf("critical mask=%d want 8", got)
	}
	infoWarning := map[string]interface{}{
		conditionLevelInfo:    struct{}{},
		conditionLevelWarning: struct{}{},
	}
	if got := conditionLevelAllowMask(infoWarning); got != 7 {
		t.Fatalf("info+warning mask=%d want 7", got)
	}
}

func TestConditionWarnCountFromFlags(t *testing.T) {
	if got := conditionWarnCountFromFlags(0); got != 0 {
		t.Fatalf("empty=%d", got)
	}
	if got := conditionWarnCountFromFlags(flagSitting); got != 0 {
		t.Fatalf("sitting=%d", got)
	}
	if got := conditionWarnCountFromFlags(flagDirty | flagSitting); got != 1 {
		t.Fatalf("dirty+sitting=%d", got)
	}
	if got := conditionWarnCountFromFlags(flagDirty | flagOverweight); got != 2 {
		t.Fatalf("two=%d", got)
	}
	if got := conditionWarnCountFromFlags(flagDirty | flagOverweight | flagBroken); got != 3 {
		t.Fatalf("three=%d", got)
	}
}

func TestMemberMatchesAllowMask(t *testing.T) {
	infoRec := conditionRecord{
		Timestamp: time.Unix(1, 0),
		Condition: packedConditionCSV[0],
		Message:   "m",
	}
	warnRec := conditionRecord{
		Timestamp: time.Unix(1, 0),
		Condition: packedConditionCSV[3],
		Message:   "m",
	}
	critRec := conditionRecord{
		Timestamp: time.Unix(1, 0),
		IsSitting: true,
		Condition: packedConditionCSV[7],
		Message:   "m",
	}
	infoM, err := encodeConditionMember(infoRec)
	if err != nil {
		t.Fatal(err)
	}
	warnM, err := encodeConditionMember(warnRec)
	if err != nil {
		t.Fatal(err)
	}
	critM, err := encodeConditionMember(critRec)
	if err != nil {
		t.Fatal(err)
	}
	infoMask := conditionLevelAllowMask(map[string]interface{}{conditionLevelInfo: struct{}{}})
	warnMask := conditionLevelAllowMask(map[string]interface{}{conditionLevelWarning: struct{}{}})
	critMask := conditionLevelAllowMask(map[string]interface{}{conditionLevelCritical: struct{}{}})
	if !memberMatchesAllowMask(infoM, infoMask) || memberMatchesAllowMask(warnM, infoMask) || memberMatchesAllowMask(critM, infoMask) {
		t.Fatal("info mask")
	}
	if memberMatchesAllowMask(infoM, warnMask) || !memberMatchesAllowMask(warnM, warnMask) || memberMatchesAllowMask(critM, warnMask) {
		t.Fatal("warning mask")
	}
	if memberMatchesAllowMask(infoM, critMask) || memberMatchesAllowMask(warnM, critMask) || !memberMatchesAllowMask(critM, critMask) {
		t.Fatal("critical mask")
	}
	if memberMatchesAllowMask("short", infoMask) {
		t.Fatal("short member")
	}
}
