// Package webui embeds the built React dashboard (Vite output in dist/) and
// serves it as a single-page app. Before the frontend is built, dist/ contains
// only .gitkeep and the handler serves a friendly placeholder.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const placeholder = `<!doctype html><html><body style="font-family:sans-serif;padding:2rem">
<h1>SupportSentinel</h1><p>Dashboard not built yet. Run <code>make frontend</code> (or <code>make build</code>) and restart.</p>
</body></html>`

// Handler serves the embedded SPA: real files when present, with a fallback to
// index.html for client-side routes; a placeholder when the app isn't built.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	indexHTML, indexErr := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Not built yet → placeholder.
		if indexErr != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(placeholder))
			return
		}
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			serveIndex(w, indexHTML)
			return
		}
		// If the requested file exists in the build, serve it; otherwise SPA fallback.
		if f, err := sub.Open(clean); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, indexHTML)
	})
}

func serveIndex(w http.ResponseWriter, indexHTML []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}
