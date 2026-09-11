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
	w.Header().Set("Cache-Control", "no-cache")

	var webPath string
	if p == "/ui" || p == "/" {
		webPath = "web/index.html"
	} else if strings.HasPrefix(p, "/ui/") {
		clean := filepath.Clean(strings.TrimPrefix(p, "/ui"))
		if strings.Contains(clean, "..") {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
			return
		}
		webPath = filepath.Join("web", clean)
	} else if strings.HasPrefix(p, "/assets/") {
		clean := filepath.Clean(p)
		if strings.Contains(clean, "..") {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
			return
		}
		webPath = filepath.Join("web", clean)
	} else if p == "/favicon.svg" || p == "/favicon.ico" {
		webPath = "web/favicon.svg"
	} else {
		webPath = "web/index.html"
	}

	http.ServeFile(w, r, webPath)
}
