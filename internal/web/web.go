package web

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var files embed.FS

func Handler() http.Handler {
	root, _ := fs.Sub(files, "dist")
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(root, name); err != nil {
			// Hashed assets must 404 when missing: an HTML fallback breaks stale tabs after an upgrade with a MIME error.
			if strings.HasPrefix(name, "api/") || strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			if _, err = fs.Stat(root, "index.html"); err != nil {
				http.Error(w, "Web assets missing: run npm ci && npm run build in web/", http.StatusServiceUnavailable)
				return
			}
			fileServer.ServeHTTP(w, r2)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			if _, err := fs.Stat(root, name+".gz"); err == nil {
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Vary", "Accept-Encoding")
				w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/" + name + ".gz"
				fileServer.ServeHTTP(w, r2)
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
