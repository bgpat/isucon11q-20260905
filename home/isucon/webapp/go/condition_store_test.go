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
	member := encodeConditionMember(rec)
	if strings.Count(member, "\x1e") != 3 {
		t.Fatalf("member separators=%d want 3: %q", strings.Count(member, "\x1e"), member)
	}
	got, err := decodeConditionMember(member, float64(rec.Timestamp.Unix()))
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSitting != rec.IsSitting || got.Condition != rec.Condition || got.Message != rec.Message {
		t.Fatalf("got=%+v want=%+v", got, rec)
	}
	if got.Timestamp.Unix() != rec.Timestamp.Unix() {
		t.Fatalf("timestamp=%d want %d", got.Timestamp.Unix(), rec.Timestamp.Unix())
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

func TestDefaultValkeyAddrUsesEnv(t *testing.T) {
	t.Setenv("VALKEY_ADDR", "example:6380")
	if got := defaultValkeyAddr(); got != "example:6380" {
		t.Fatalf("addr=%q", got)
	}
}

func TestDefaultValkeyAddrUsesIsucon2Host(t *testing.T) {
	t.Setenv("VALKEY_ADDR", "")
	got := defaultValkeyAddr()
	if hostnameIs(isucon2Hostname) {
		if got != "127.0.0.1:6379" {
			t.Fatalf("addr=%q", got)
		}
		return
	}
	if got != isucon2Hostname+":6379" {
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
	a := encodeConditionMember(rec)
	b := encodeConditionMember(rec)
	if a == b {
		t.Fatalf("members should differ: %q", a)
	}
}
