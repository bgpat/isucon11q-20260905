package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestCachedIconRememberAndInvalidate(t *testing.T) {
	invalidateCachedIcon()
	t.Cleanup(invalidateCachedIcon)

	const user = "user-a"
	const uuid = "11111111-1111-1111-1111-111111111111"
	if hasCachedIcon(user, uuid) {
		t.Fatal("cache should start empty")
	}
	rememberCachedIcon(user, uuid)
	if !hasCachedIcon(user, uuid) {
		t.Fatal("rememberCachedIcon should store key")
	}
	if hasCachedIcon("user-b", uuid) {
		t.Fatal("other user should not share the key")
	}
	invalidateCachedIcon()
	if hasCachedIcon(user, uuid) {
		t.Fatal("invalidateCachedIcon should clear key")
	}
}

func TestServeCachedIsuIconSkipsStatWhenRemembered(t *testing.T) {
	invalidateCachedIcon()
	t.Cleanup(invalidateCachedIcon)

	const user = "user-a"
	const uuid = "11111111-1111-1111-1111-111111111111"
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if serveCachedIsuIcon(c, user, uuid) {
		t.Fatal("missing file and memory should miss")
	}

	rememberCachedIcon(user, uuid)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req, rec2)
	if !serveCachedIsuIcon(c2, user, uuid) {
		t.Fatal("remembered key should hit without Stat")
	}
	got := rec2.Header().Get("X-Accel-Redirect")
	want := isuIconInternalPath + filepath.Base(isuIconCachePath(user, uuid))
	if got != want {
		t.Fatalf("X-Accel-Redirect=%q want %q", got, want)
	}
}
