// Package web serves the static Cinder frontend embedded into the binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed dist
var distFS embed.FS

var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".svg":  "image/svg+xml",
	".txt":  "text/plain; charset=utf-8",
}

// Handler serves GET / (create page), GET /s/{id} (the identical view shell for
// every id, with no store access) and GET /static/* assets.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", page("dist/index.html"))
	mux.HandleFunc("GET /s/{id}", page("dist/view.html"))
	mux.HandleFunc("GET /static/", asset)
	return noStore(mux)
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		next.ServeHTTP(w, r)
	})
}

func serve(w http.ResponseWriter, r *http.Request, name string) {
	b, err := fs.ReadFile(distFS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct, ok := contentTypes[path.Ext(name)]
	if !ok {
		ct = "application/octet-stream"
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(b)
	}
}

func page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { serve(w, r, name) }
}

func asset(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/static/")
	if rel == "" || strings.HasSuffix(rel, "/") || !fs.ValidPath(rel) {
		http.NotFound(w, r)
		return
	}
	serve(w, r, "dist/static/"+rel)
}
