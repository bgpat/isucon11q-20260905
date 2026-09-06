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

func TestConditionDescFetchCount(t *testing.T) {
	if got := conditionDescFetchCount(1); got != 64 {
		t.Fatalf("small limit fetch=%d want 64", got)
	}
	if got := conditionDescFetchCount(20); got != 80 {
		t.Fatalf("limit 20 fetch=%d want 80", got)
	}
	if got := conditionDescFetchCount(160); got != 640 {
		t.Fatalf("limit 160 fetch=%d want 640", got)
	}
}

func TestOffsetPaginationKeepsSameSecondLeftovers(t *testing.T) {
	// graph は同じ秒を全部出す。timestamp カーソルだと LIMIT で途切れた同じ秒が次ページから消える。
	desc := []int64{100, 100, 100, 99}
	endExclusive := int64(101)
	fetch := 2

	takePage := func(maxExclusive int64, offset int) []int64 {
		eligible := make([]int64, 0, len(desc))
		for _, ts := range desc {
			if ts < maxExclusive {
				eligible = append(eligible, ts)
			}
		}
		if offset >= len(eligible) {
			return nil
		}
		end := offset + fetch
		if end > len(eligible) {
			end = len(eligible)
		}
		return eligible[offset:end]
	}

	var byTimestamp []int64
	cursor := endExclusive
	for {
		page := takePage(cursor, 0)
		if len(page) == 0 {
			break
		}
		byTimestamp = append(byTimestamp, page...)
		if len(page) < fetch {
			break
		}
		cursor = page[len(page)-1]
	}
	if len(byTimestamp) == len(desc) {
		t.Fatalf("timestamp cursor should drop a same-second leftover: %v", byTimestamp)
	}

	var byOffset []int64
	offset := 0
	for {
		page := takePage(endExclusive, offset)
		if len(page) == 0 {
			break
		}
		byOffset = append(byOffset, page...)
		if len(page) < fetch {
			break
		}
		offset += len(page)
	}
	if len(byOffset) != len(desc) {
		t.Fatalf("offset pagination got %v want %v", byOffset, desc)
	}
	for i := range desc {
		if byOffset[i] != desc[i] {
			t.Fatalf("offset pagination got %v want %v", byOffset, desc)
		}
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
