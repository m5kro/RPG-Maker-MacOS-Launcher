package ui

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assetFixture(t *testing.T) (string, string) {
	t.Helper()
	web, res := t.TempDir(), t.TempDir()
	for _, name := range []string{"index.html", "app.js", "style.css"} {
		if err := os.WriteFile(filepath.Join(web, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"bg.js", "disable-child.js", "disable-net.js"} {
		if err := os.WriteFile(filepath.Join(res, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return web, res
}
func TestAssetPathsAndNoBundleFallback(t *testing.T) {
	web, res := assetFixture(t)
	if _, err := ResolvePaths("/tmp/source-launcher", web, res); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolvePaths("/tmp/source-launcher", "relative", res); err == nil {
		t.Fatal("relative assets")
	}
	if _, err := ResolvePaths(filepath.Join(t.TempDir(), "Broken.app/Contents/MacOS/launcher"), web, res); err == nil {
		t.Fatal("bundle fell back to source")
	}
	if err := os.Remove(filepath.Join(web, "app.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(res, "bg.js"), filepath.Join(web, "app.js")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolvePaths("/tmp/source-launcher", web, res); err == nil {
		t.Fatal("symlink asset")
	}
}
func TestAssetServerOnlyServesUI(t *testing.T) {
	web, _ := assetFixture(t)
	if err := os.WriteFile(filepath.Join(web, "secret"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := AssetHandler(web)
	for _, path := range []string{"/", "/app.js", "/style.css"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 || !strings.Contains(r.Header().Get("Content-Security-Policy"), "form-action 'none'") {
			t.Fatalf("%s %d %v", path, r.Code, r.Header())
		}
	}
	for _, path := range []string{"/secret", "/../secret", "/game.log", "/settings.json"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 404 {
			t.Fatalf("%s = %d", path, r.Code)
		}
	}
}
