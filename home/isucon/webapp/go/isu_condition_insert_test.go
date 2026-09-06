package main

import (
	"strings"
	"testing"
)

func TestBuildIsuConditionInsertValues(t *testing.T) {
	got := buildIsuConditionInsertValues(3)
	want := "(?, ?, ?, ?, ?),(?, ?, ?, ?, ?),(?, ?, ?, ?, ?)"
	if got != want {
		t.Fatalf("placeholders=%q want %q", got, want)
	}
	if strings.Contains(got, "UNION ALL") {
		t.Fatalf("VALUES placeholders must not contain UNION ALL: %q", got)
	}
	placeholderCount := strings.Count(got, "?")
	if placeholderCount != 15 {
		t.Fatalf("placeholder count=%d want 15", placeholderCount)
	}
}

func TestBuildIsuConditionInsertValuesOneRow(t *testing.T) {
	got := buildIsuConditionInsertValues(1)
	if got != "(?, ?, ?, ?, ?)" {
		t.Fatalf("placeholders=%q", got)
	}
}

func TestIsuUUIDCacheRememberAndInvalidate(t *testing.T) {
	invalidateIsuUUIDCache()
	defer invalidateIsuUUIDCache()

	const uuid = "11111111-1111-1111-1111-111111111111"
	if _, ok := isuUUIDCache.Load(uuid); ok {
		t.Fatal("cache should start empty")
	}
	rememberIsuUUID(uuid)
	if _, ok := isuUUIDCache.Load(uuid); !ok {
		t.Fatal("rememberIsuUUID should store uuid")
	}
	invalidateIsuUUIDCache()
	if _, ok := isuUUIDCache.Load(uuid); ok {
		t.Fatal("invalidateIsuUUIDCache should clear uuid")
	}
}
