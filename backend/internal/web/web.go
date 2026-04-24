// Package web serves the embedded Vite build of the frontend.
//
// Build flow:
//  1. pnpm --dir frontend build → frontend/dist
//  2. copy frontend/dist → backend/internal/web/dist
//  3. go build ./cmd/gateway → single binary with UI + API
//
// The Makefile at repo root automates steps 1–3.
package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded SPA.
// Unknown non-API paths fall back to index.html so the Vue router can resolve
// them client-side.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Build system guarantees dist/ exists; a bad embed is a compile-time bug.
		panic("web: embedded dist missing: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	index, _ := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath == "" {
			serveIndex(w, index)
			return
		}
		// If the requested file exists in the bundle, serve it directly.
		if f, err := sub.Open(reqPath); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				// Cache hashed vite assets aggressively.
				if strings.HasPrefix(reqPath, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Static extension miss → 404; HTML route → SPA fallback.
		if ext := path.Ext(reqPath); ext != "" && ext != ".html" {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, index)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, strings.NewReader(string(index)))
}
