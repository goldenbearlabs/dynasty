package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist holds the SvelteKit build, copied in by `make build` and the Dockerfile.
//
//go:embed all:dist
var dist embed.FS

// frontend serves the built single-page app. Any path that is not a built
// file gets index.html so the client-side router can handle it.
func frontend() http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(root, path); path == "" || err != nil {
			http.ServeFileFS(w, r, root, "index.html")
			return
		}
		if strings.HasPrefix(path, "_app/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
