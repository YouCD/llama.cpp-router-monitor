package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/_monitor") {
		s.handleMonitor(w, r)
		return
	}
	s.handleProxy(w, r)
}
func (s *Server) handleMonitor(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/_monitor")
	if p == "" {
		p = "/"
	}

	switch {
	case p == "/":
		writeJSON(w, http.StatusOK, map[string]any{
			"name":    "llama.cpp Router Monitor",
			"version": "1.0.0",
			"endpoints": []string{
				"/_monitor/health",
				"/_monitor/live",
				"/_monitor/stats?hours=24",
				"/_monitor/requests?limit=100&offset=0",
				"/_monitor/request/{id}",
				"/_monitor/raw/{id}/{request|response}",
				"/_monitor/events",
				"/_monitor/backend-metrics?limit=200",
				"/_monitor/daily-stats?days=30",
				"/_monitor/ui",
			},
		})
	case p == "/ui" || strings.HasPrefix(p, "/ui/"):
		s.handleUI(w, r, p)
	case p == "/health":
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC(), "active_connections": s.active.Load()})
	case p == "/stats":
		hours := getQueryInt(r, "hours", 24)
		f := parseRequestFilter(r)
		stats, err := s.getStats(hours, f)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, stats)
	case p == "/models":
		items, err := s.getModels()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case p == "/backends":
		items, err := s.getBackends()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case p == "/stats-by-backend":
		hours := getQueryInt(r, "hours", 24)
		items, err := s.getStatsByBackend(hours)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "hours": hours})
	case p == "/daily-stats":
		days := getQueryInt(r, "days", 30)
		f := parseRequestFilter(r)
		items, err := s.getDailyStats(days, f)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "days": days})
	case p == "/requests":
		limit := getQueryInt(r, "limit", 100)
		offset := getQueryInt(r, "offset", 0)
		f := parseRequestFilter(r)

		recs, err := s.getRequests(limit, offset, f)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": recs, "limit": limit, "offset": offset, "filters": f})
	case p == "/events":
		s.handleEvents(w, r)
	case p == "/live":
		writeJSON(w, http.StatusOK, map[string]any{"active_connections": s.active.Load(), "time": time.Now().UTC()})
	case p == "/backend-metrics":
		limit := getQueryInt(r, "limit", 200)
		items, err := s.getBackendMetrics(limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit})
	case strings.HasPrefix(p, "/request/"):
		id := strings.TrimPrefix(p, "/request/")
		if r.Method == http.MethodDelete {
			if err := s.deleteRequestByID(id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			s.hub.Broadcast(map[string]any{"kind": "request_deleted", "id": id, "time": time.Now().UTC()})
			writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "id": id})
			return
		}
		rec, err := s.getRequestByID(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case strings.HasPrefix(p, "/raw/"):
		s.handleRaw(w, p)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown monitor endpoint"})
	}
}
func parseRequestFilter(r *http.Request) RequestFilter {
	f := RequestFilter{
		Path:                strings.TrimSpace(r.URL.Query().Get("path")),
		Model:               strings.TrimSpace(r.URL.Query().Get("model")),
		Method:              strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("method"))),
		Backend:             strings.TrimSpace(r.URL.Query().Get("backend")),
		Search:              strings.TrimSpace(r.URL.Query().Get("q")),
		StatusCode:          getQueryInt(r, "status", 0),
		SinceHours:          getQueryInt(r, "since_hours", 0),
		ErrorsOnly:          getQueryBool(r, "errors_only", false),
		WithTokens:          getQueryBool(r, "with_tokens", false),
		ChatCompletionsOnly: getQueryBool(r, "chat_completions_only", false),
	}
	if stream, ok := getOptionalQueryBool(r, "stream"); ok {
		f.Streaming = &stream
	}
	return f
}
func (s *Server) handleRaw(w http.ResponseWriter, p string) {
	parts := strings.Split(strings.TrimPrefix(p, "/raw/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "use /_monitor/raw/{request_id}/{request|response}"})
		return
	}

	id := parts[0]
	kind := parts[1]
	rec, err := s.getRequestByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var rel string
	if kind == "request" {
		rel = rec.RequestRawPath
	} else if kind == "response" {
		rel = rec.ResponseRawPath
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "part must be request or response"})
		return
	}
	if rel == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "raw payload not available"})
		return
	}

	fullPath := filepath.Clean(filepath.Join(s.cfg.DataDir, rel))
	data, err := readGzipFile(fullPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if json.Valid(data) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Header().Set("X-Request-ID", id)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx, cancelSSE := s.newSSECtx(r.Context())
	defer cancelSSE()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			_, _ = fmt.Fprintf(w, "event: request\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}

// handleModels 让路由器自身应答 OpenAI 兼容的 GET /v1/models。
// 固定返回一个占位模型 llm_prox，供客户端作为可用模型 ID 使用；
// 实际转发时仍走负载均衡，接受客户端提交的任意模型名。不转发到后端，也不记录。
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	type obj struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []obj{
			{ID: "llm_prox", Object: "model", Created: time.Now().Unix(), OwnedBy: "llama-cpp-router-monitor"},
		},
	})
}
