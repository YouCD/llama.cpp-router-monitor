//go:build embedweb

package main

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed web
var webFS embed.FS

// handleUI serves the frontend embedded into the binary.
// Enabled by building with `go build -tags embedweb`.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request, p string) {
	rel := strings.TrimPrefix(p, "/ui")
	w.Header().Set("Cache-Control", "no-cache")

	name := "web/index.html"
	if rel != "" && rel != "/" {
		clean := path.Clean(strings.TrimPrefix(rel, "/"))
		if strings.Contains(clean, "..") {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
			return
		}
		name = path.Join("web", clean)
	}

	data, err := fs.ReadFile(webFS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	_, _ = w.Write(data)
}
