// Package webui serves the embedded React single-page app. The real assets are
// written to dist/ by `npm --prefix web run build`; only dist/.gitkeep is
// committed, so before a frontend build the handler serves a short placeholder
// page instead of the SPA.
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

const placeholder = `<!doctype html><meta charset="utf-8"><title>ClamAV WebUI</title>
<body style="font-family:system-ui;margin:3rem;max-width:40rem">
<h1>ClamAV WebUI</h1>
<p>The backend is running, but the web frontend has not been built into this
binary. Build it with <code>npm --prefix web install &amp;&amp; npm --prefix web run build</code>
(or <code>make build</code>) and restart.</p>
</body>`

// Handler returns an http.Handler serving the SPA: real files when present,
// otherwise index.html as the client-routing fallback. Unknown /assets/ paths
// get a 404. When no build is embedded, every non-asset path gets the
// placeholder page.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("webui: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	index, indexErr := fs.ReadFile(sub, "index.html")
	if indexErr != nil {
		index = []byte(placeholder)
	}
	start := time.Now()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		rel := strings.TrimPrefix(clean, "/")

		if rel != "" && rel != "index.html" {
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
