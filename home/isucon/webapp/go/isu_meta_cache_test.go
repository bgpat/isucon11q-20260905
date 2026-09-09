package main

import "testing"

func TestIsuMetaCacheStoreListAndInvalidate(t *testing.T) {
	invalidateIsuMetaCache()
	invalidateIsuUUIDCache()
	t.Cleanup(func() {
		invalidateIsuMetaCache()
		invalidateIsuUUIDCache()
	})

	a := isuMeta{ID: 2, JIAIsuUUID: "u-2", Name: "two", Character: "cat", JIAUserID: "user-a"}
	b := isuMeta{ID: 1, JIAIsuUUID: "u-1", Name: "one", Character: "dog", JIAUserID: "user-a"}
	other := isuMeta{ID: 3, JIAIsuUUID: "u-3", Name: "oth", Character: "cat", JIAUserID: "user-b"}

	if _, ok := isuMetas.list("user-a"); ok {
		t.Fatal("unloaded user should miss")
	}

	isuMetas.storeUser("user-a", []isuMeta{a, b})
	isuMetas.store(other)

	got, ok := isuMetas.list("user-a")
	if !ok {
		t.Fatal("loaded user should hit")
	}
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 {
		t.Fatalf("list=%+v", got)
	}
	if _, ok := isuUUIDCache.Load("u-2"); !ok {
		t.Fatal("storeUser should remember uuid")
	}

	empty, ok := isuMetas.list("user-empty")
	if ok {
		t.Fatalf("unloaded empty user should miss: %+v", empty)
	}
	isuMetas.storeUser("user-empty", nil)
	empty, ok = isuMetas.list("user-empty")
	if !ok || len(empty) != 0 {
		t.Fatalf("loaded empty user: ok=%v list=%+v", ok, empty)
	}

	invalidateIsuMetaCache()
	if _, ok := isuMetas.get("u-2"); ok {
		t.Fatal("expected miss after invalidate")
	}
	if _, ok := isuMetas.list("user-a"); ok {
		t.Fatal("loaded flag should clear")
	}
}

func TestForgetIsuMetaRemovesUUIDKeepUserLoaded(t *testing.T) {
	invalidateIsuMetaCache()
	invalidateIsuUUIDCache()
	t.Cleanup(func() {
		invalidateIsuMetaCache()
		invalidateIsuUUIDCache()
	})

	keep := isuMeta{ID: 2, JIAIsuUUID: "keep", Name: "k", Character: "cat", JIAUserID: "user-a"}
	drop := isuMeta{ID: 1, JIAIsuUUID: "drop", Name: "d", Character: "dog", JIAUserID: "user-a"}
	isuMetas.storeUser("user-a", []isuMeta{keep, drop})
	forgetIsuUUID("drop")
	forgetIsuMeta("drop")

	if _, ok := isuMetas.get("drop"); ok {
		t.Fatal("forgotten uuid should miss")
	}
	if _, ok := isuUUIDCache.Load("drop"); ok {
		t.Fatal("forgotten uuid cache should miss")
	}
	got, ok := isuMetas.list("user-a")
	if !ok {
		t.Fatal("user list should stay loaded")
	}
	if len(got) != 1 || got[0].JIAIsuUUID != "keep" {
		t.Fatalf("list=%+v", got)
	}
}

func TestIsuMetaCacheDoesNotMarkUserLoadedOnSingleStore(t *testing.T) {
	invalidateIsuMetaCache()
	t.Cleanup(invalidateIsuMetaCache)

	rememberIsuMeta(isuMeta{ID: 9, JIAIsuUUID: "new", Name: "n", Character: "c", JIAUserID: "user-a"})
	if _, ok := isuMetas.list("user-a"); ok {
		t.Fatal("single store must not mark the user list as complete")
	}
	if _, ok := isuMetas.get("new"); !ok {
		t.Fatal("single store should still keep the row")
	}
}
