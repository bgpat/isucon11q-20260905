package main

import (
	"testing"
)

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
