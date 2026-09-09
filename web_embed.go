//go:build embedweb

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
)

//go:embed web
var webFS embed.FS

// handleUI serves the frontend embedded into the binary.
// Enabled by building with `go build -tags embedweb`.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request, p string) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var name string
	if p == "/ui" || p == "/" {
		name = "web/index.html"
	} else if strings.HasPrefix(p, "/ui/") {
		clean := path.Clean(strings.TrimPrefix(p, "/ui"))
		if strings.Contains(clean, "..") {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
			return
		}
		name = path.Join("web", clean)
	} else if strings.HasPrefix(p, "/assets/") {
		clean := path.Clean(p)
		if strings.Contains(clean, "..") {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid path"})
			return
		}
		name = path.Join("web", clean)
	} else {
		name = "web/index.html"
	}

	data, err := fs.ReadFile(webFS, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "handleUI: failed to read %s: %v\n", name, err)
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	_, _ = w.Write(data)
}
