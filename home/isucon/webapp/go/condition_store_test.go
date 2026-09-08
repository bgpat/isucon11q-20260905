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
