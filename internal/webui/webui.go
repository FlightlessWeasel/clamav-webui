// Package webui serves the embedded React single-page app. The real assets are
// written to dist/ by `npm --prefix web run build`; a placeholder index.html is
// committed so the Go build works before the frontend exists.
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the SPA: real files when they
// exist, otherwise index.html (client-side routing fallback). Unknown paths
// under /assets/ get a 404 rather than the HTML shell.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("webui: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic("webui: missing dist/index.html: " + err.Error())
	}
	start := time.Now()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		rel := strings.TrimPrefix(clean, "/")

		if rel != "" {
			if f, err := sub.Open(rel); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(clean, "/assets/") {
				http.NotFound(w, r)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", start, bytes.NewReader(index))
	})
}
