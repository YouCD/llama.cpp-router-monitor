//go:build !embedweb

package main

import (
	"net/http"
	"path/filepath"
	"strings"
)

// handleUI serves the frontend from the on-disk web/ directory.
// Use `go build -tags embedweb` to embed the frontend into the binary instead.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request, p string) {
	rel := strings.TrimPrefix(p, "/ui")
	w.Header().Set("Cache-Control", "no-cache")
	if rel == "" || rel == "/" {
		http.ServeFile(w, r, filepath.Join("web", "index.html"))
		return
	}
	clean := filepath.Clean(strings.TrimPrefix(rel, "/"))
	if strings.Contains(clean, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
		return
	}
	http.ServeFile(w, r, filepath.Join("web", clean))
}
