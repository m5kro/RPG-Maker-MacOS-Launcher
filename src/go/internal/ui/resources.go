package ui

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Paths struct{ WebUI, Resources string }

func ResolvePaths(executable, web, resources string) (Paths, error) {
	if !filepath.IsAbs(executable) {
		return Paths{}, errors.New("executable must be absolute")
	}
	parent := filepath.Dir(executable)
	if filepath.Base(parent) == "MacOS" && filepath.Base(filepath.Dir(parent)) == "Contents" {
		resources = filepath.Join(filepath.Dir(parent), "Resources")
		web = filepath.Join(resources, "webui")
	}
	p := Paths{web, resources}
	for _, dir := range []string{p.WebUI, p.Resources} {
		if !filepath.IsAbs(dir) {
			return p, errors.New("source launch requires absolute --webui and --resources")
		}
		if _, err := absoluteDirectory(dir); err != nil {
			return p, err
		}
	}
	for _, file := range []string{filepath.Join(web, "index.html"), filepath.Join(web, "app.js"), filepath.Join(web, "style.css"), filepath.Join(resources, "bg.js"), filepath.Join(resources, "disable-child.js"), filepath.Join(resources, "disable-net.js")} {
		info, err := os.Lstat(file)
		if err != nil {
			return p, err
		}
		if !info.Mode().IsRegular() {
			return p, fmt.Errorf("asset must be a regular file: %s", file)
		}
	}
	return p, nil
}

const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-src 'none'; frame-ancestors 'none'"

func AssetHandler(web string) http.Handler {
	files := application.AssetFileServerFS(os.DirFS(web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		switch r.URL.Path {
		case "/", "/index.html", "/app.js", "/style.css":
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		files.ServeHTTP(w, r)
	})
}
