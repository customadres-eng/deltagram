package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// Собранный фронт лежит в web/dist и вшивается в бинарник.
// В репозитории есть заглушка index.html, Docker и npm build перезаписывают её.
//
//go:embed web/dist
var embeddedWeb embed.FS

// отдаём собранный фронт, все неизвестные пути на index.html
func withStatic(api http.Handler, dir string) http.Handler {
	var web fs.FS
	if dir != "" {
		web = os.DirFS(dir)
	} else {
		sub, err := fs.Sub(embeddedWeb, "web/dist")
		if err != nil {
			return api
		}
		web = sub
	}
	index, err := fs.ReadFile(web, "index.html")
	if err != nil {
		return api
	}
	files := http.FileServer(http.FS(web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			api.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		st, err := fs.Stat(web, name)
		if err != nil || st.IsDir() {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(index)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
