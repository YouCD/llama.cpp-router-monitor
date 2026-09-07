package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/models" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		s.handleModels(w, r)
		return
	}
	if !s.shouldRecordProxy(r.URL.Path) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	started := time.Now()
	requestID := newID()
	clientIP := getClientIP(r)
	backendURL, trimmedQuery, backendCfg, err := s.selectBackend(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	if s.cfg.MaxRequestBytes > 0 && r.ContentLength > s.cfg.MaxRequestBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request is too large"})
		return
	}

	requestBody, err := readWithLimit(r.Body, s.cfg.MaxRequestBytes)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, errTooLarge) {
			code = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}

	isStreaming, model := detectRequestMeta(requestBody)
	if backendCfg != nil && backendCfg.Model != "" {
		if newBody, newModel, rerr := rewriteModel(requestBody, backendCfg.Model); rerr == nil {
			requestBody = newBody
			model = newModel
		}
	}
	reqRawPath, err := s.saveRawPayload(requestID, "request", requestBody)
	if err != nil {
		log.Printf("save request raw failed: %v", err)
	}

	if err := s.insertRequest(RequestRecord{
		ID:             requestID,
		CreatedAt:      started.UTC(),
		Method:         r.Method,
		Path:           r.URL.Path,
		Query:          trimmedQuery,
		ClientIP:       clientIP,
		BackendURL:     backendURL,
		Model:          model,
		IsStreaming:    isStreaming,
		RequestBytes:   int64(len(requestBody)),
		RequestRawPath: reqRawPath,
		UserAgent:      r.UserAgent(),
	}); err != nil {
		log.Printf("insert request failed: %v", err)
	}

	target := backendURL + r.URL.Path
	if trimmedQuery != "" {
		target += "?" + trimmedQuery
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(requestBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	copyRequestHeaders(outReq.Header, r.Header)
	if backendCfg != nil && backendCfg.APIKey != "" {
		outReq.Header.Set("Authorization", "Bearer "+backendCfg.APIKey)
	}
	outReq.Header.Set("X-Proxy-Request-ID", requestID)
	outReq.ContentLength = int64(len(requestBody))

	s.active.Add(1)
	s.hub.Broadcast(map[string]any{"kind": "active", "active_connections": s.active.Load(), "time": time.Now().UTC()})
	defer func() {
		s.active.Add(-1)
		s.hub.Broadcast(map[string]any{"kind": "active", "active_connections": s.active.Load(), "time": time.Now().UTC()})
	}()

	resp, err := s.client.Do(outReq)
	if err != nil {
		total := float64(time.Since(started).Milliseconds())
		_ = s.finishRequest(requestID, RequestRecord{StatusCode: 502, ErrorText: err.Error(), TotalMs: total})
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "request_id": requestID})
		return
	}
	defer resp.Body.Close()

	for k, vals := range resp.Header {
		if isHopByHopHeader(k) {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Proxy-Request-ID", requestID)
	w.WriteHeader(resp.StatusCode)

	var firstByteMs float64
	var copied int64
	var chunks int64
	capturer := newLimitedBuffer(s.cfg.MaxCaptureBytes)

	if isStreamingResponse(resp, isStreaming) {
		copied, firstByteMs, chunks, err = streamCopySSE(w, resp.Body, capturer, started)
	} else {
		copied, firstByteMs, err = streamCopy(w, resp.Body, capturer, started)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("copy response failed req=%s: %v", requestID, err)
	}

	respBytes := capturer.Bytes()
	respRawPath, saveErr := s.saveRawPayload(requestID, "response", respBytes)
	if saveErr != nil {
		log.Printf("save response raw failed: %v", saveErr)
	}

	meta := parseResponseMeta(resp.Header, respBytes)
	finalModel := model
	if meta.Model != "" {
		finalModel = meta.Model
	}
	cacheHitPct := 0.0
	if meta.PromptTokens > 0 && meta.CachedPromptTokens > 0 {
		cacheHitPct = float64(meta.CachedPromptTokens) / float64(meta.PromptTokens) * 100
	}
	totalMs := float64(time.Since(started).Milliseconds())

	if err := s.finishRequest(requestID, RequestRecord{
		Model:              finalModel,
		StatusCode:         resp.StatusCode,
		ResponseBytes:      copied,
		PromptTokens:       meta.PromptTokens,
		CachedPromptTokens: meta.CachedPromptTokens,
		CacheHitPct:        cacheHitPct,
		CompletionTokens:   meta.CompletionTok,
		TotalTokens:        meta.TotalTokens,
		PromptMs:           meta.PromptMs,
		CompletionMs:       meta.CompletionMs,
		TotalMs:            totalMs,
		FirstByteMs:        firstByteMs,
		ChunksCount:        chunks,
		ResponseRawPath:    respRawPath,
	}); err != nil {
		log.Printf("finish request failed: %v", err)
	}

	s.hub.Broadcast(map[string]any{
		"kind":                 "request",
		"id":                   requestID,
		"time":                 time.Now().UTC(),
		"path":                 r.URL.Path,
		"method":               r.Method,
		"status_code":          resp.StatusCode,
		"total_ms":             totalMs,
		"first_byte_ms":        firstByteMs,
		"prompt_tokens":        meta.PromptTokens,
		"cached_prompt_tokens": meta.CachedPromptTokens,
		"cache_hit_pct":        cacheHitPct,
		"completion_tokens":    meta.CompletionTok,
		"total_tokens":         meta.TotalTokens,
		"model":                finalModel,
		"response_bytes":       copied,
		"chunks_count":         chunks,
		"backend_url":          backendURL,
		"active_connections":   s.active.Load(),
	})
}
func (s *Server) pathMatches(path string, list []string) bool {
	p := strings.Trim(path, "/")
	if p == "" {
		return false
	}
	for _, ig := range list {
		raw := strings.Trim(ig, "/")
		if raw == "" {
			continue
		}
		if p == raw || strings.HasPrefix(p, raw+"/") {
			return true
		}
	}
	return false
}

// shouldRecordProxy 决定某个路径的请求是否需要转发并记录。
// 仅命中白名单 record_paths 的路径会被转发记录；其余一律 404。
func (s *Server) shouldRecordProxy(path string) bool {
	if path == "/" {
		return false
	}
	return s.pathMatches(path, s.cfg.RecordPaths)
}

func (s *Server) selectBackend(r *http.Request) (backend string, query string, bc *BackendConfig, err error) {
	vals := r.URL.Query()

	if s.cfg.AllowDynamicBackend {
		if b := strings.TrimSpace(r.Header.Get("X-Backend-URL")); b != "" {
			backend = strings.TrimRight(b, "/")
			bc = nil
		} else if b := strings.TrimSpace(vals.Get("backend")); b != "" {
			backend = strings.TrimRight(b, "/")
			vals.Del("backend")
			bc = nil
		} else if s.balancer != nil {
			if b := s.balancer.Select(); b != nil {
				backend = strings.TrimRight(b.URL, "/")
				bc = b
			}
		}
	} else if s.balancer != nil {
		if b := s.balancer.Select(); b != nil {
			backend = strings.TrimRight(b.URL, "/")
			bc = b
		}
	}

	if backend == "" {
		return "", "", nil, fmt.Errorf("no backend selected: configure backends.list or provide a dynamic backend override")
	}

	if err = validateBackendURL(backend); err != nil {
		return "", "", nil, fmt.Errorf("invalid backend URL: %w", err)
	}
	return backend, vals.Encode(), bc, nil
}
func isStreamingResponse(resp *http.Response, reqStreaming bool) bool {
	if reqStreaming {
		return true
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	return strings.Contains(ct, "text/event-stream")
}
func streamCopy(w http.ResponseWriter, src io.Reader, capture *limitedBuffer, started time.Time) (copied int64, firstByteMs float64, err error) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	seenFirst := false
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if !seenFirst {
				seenFirst = true
				firstByteMs = float64(time.Since(started).Milliseconds())
			}
			chunk := buf[:n]
			wn, werr := w.Write(chunk)
			if wn > 0 {
				copied += int64(wn)
				_, _ = capture.Write(chunk[:wn])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if werr != nil {
				return copied, firstByteMs, werr
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return copied, firstByteMs, nil
			}
			return copied, firstByteMs, rerr
		}
	}
}

func streamCopySSE(w http.ResponseWriter, src io.Reader, capture *limitedBuffer, started time.Time) (copied int64, firstByteMs float64, chunks int64, err error) {
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(src)
	seenFirst := false
	for {
		line, rerr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if !seenFirst {
				seenFirst = true
				firstByteMs = float64(time.Since(started).Milliseconds())
			}
			if bytes.HasPrefix(line, []byte("data:")) {
				trimmed := strings.TrimSpace(strings.TrimPrefix(string(line), "data:"))
				if trimmed != "" && trimmed != "[DONE]" {
					chunks++
				}
			}
			wn, werr := w.Write(line)
			if wn > 0 {
				copied += int64(wn)
				_, _ = capture.Write(line[:wn])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if werr != nil {
				return copied, firstByteMs, chunks, werr
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return copied, firstByteMs, chunks, nil
			}
			return copied, firstByteMs, chunks, rerr
		}
	}
}

func parseResponseMeta(headers http.Header, body []byte) responseMeta {
	var meta responseMeta
	ct := headers.Get("Content-Type")
	mediatype, _, _ := mime.ParseMediaType(ct)
	if strings.Contains(mediatype, "json") {
		return parseJSONResponseMeta(body)
	}
	if strings.Contains(mediatype, "text/event-stream") {
		parseSSEResponseMeta(body, &meta)
	}
	return meta
}

func parseJSONResponseMeta(body []byte) responseMeta {
	var meta responseMeta
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return meta
	}
	if v, ok := m["model"].(string); ok {
		meta.Model = v
	}

	if usage, ok := m["usage"].(map[string]any); ok {
		meta.PromptTokens = toInt64(usage["prompt_tokens"])
		meta.CompletionTok = toInt64(usage["completion_tokens"])
		meta.TotalTokens = toInt64(usage["total_tokens"])
		if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
			meta.CachedPromptTokens = toInt64(details["cached_tokens"])
		}
	}
	if timings, ok := m["timings"].(map[string]any); ok {
		if meta.PromptTokens == 0 {
			meta.PromptTokens = toInt64(timings["prompt_n"])
		}
		if meta.CompletionTok == 0 {
			meta.CompletionTok = toInt64(timings["predicted_n"])
		}
		meta.PromptMs = toFloat64(timings["prompt_ms"])
		meta.CompletionMs = toFloat64(timings["predicted_ms"])
	}
	if meta.PromptTokens == 0 {
		meta.PromptTokens = toInt64(m["tokens_evaluated"])
	}
	if meta.CompletionTok == 0 {
		meta.CompletionTok = toInt64(m["tokens_predicted"])
	}
	if meta.TotalTokens == 0 {
		meta.TotalTokens = meta.PromptTokens + meta.CompletionTok
	}
	if meta.PromptMs == 0 {
		meta.PromptMs = toFloat64(m["tokens_evaluated_ms"])
	}
	if meta.CompletionMs == 0 {
		meta.CompletionMs = toFloat64(m["tokens_predicted_ms"])
	}
	return meta
}

func mergeResponseMeta(dst *responseMeta, src responseMeta) {
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.PromptTokens > 0 {
		dst.PromptTokens = src.PromptTokens
	}
	if src.CachedPromptTokens > 0 {
		dst.CachedPromptTokens = src.CachedPromptTokens
	}
	if src.CompletionTok > 0 {
		dst.CompletionTok = src.CompletionTok
	}
	if src.TotalTokens > 0 {
		dst.TotalTokens = src.TotalTokens
	}
	if src.PromptMs > 0 {
		dst.PromptMs = src.PromptMs
	}
	if src.CompletionMs > 0 {
		dst.CompletionMs = src.CompletionMs
	}
}

func parseSSEResponseMeta(body []byte, meta *responseMeta) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		chunkMeta := parseJSONResponseMeta([]byte(payload))
		mergeResponseMeta(meta, chunkMeta)
	}
	if meta.TotalTokens == 0 {
		meta.TotalTokens = meta.PromptTokens + meta.CompletionTok
	}
}
func detectRequestMeta(body []byte) (isStreaming bool, model string) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return false, ""
	}
	isStreaming, _ = m["stream"].(bool)
	if v, ok := m["model"].(string); ok {
		model = v
	}
	return
}

// rewriteModel replaces the "model" field in a JSON request body with the
// backend's actual model ID. It returns the rewritten body and the new model
// name. If the body is not valid JSON (or has no model field), the original
// body is returned unchanged.
func rewriteModel(body []byte, model string) ([]byte, string, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body, "", err
	}
	m["model"] = model
	newBody, err := json.Marshal(m)
	if err != nil {
		return body, "", err
	}
	return newBody, model, nil
}
func (s *Server) backendMetricsLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollBackendMetrics(ctx)
		}
	}
}

func (s *Server) pollBackendMetrics(ctx context.Context) {
	// Collect all enabled backends (from the balancer list) to poll.
	var urls []string
	if s.balancer != nil {
		for _, name := range s.balancer.Names() {
			if b := s.balancer.GetBackendByName(name); b != nil {
				urls = append(urls, strings.TrimRight(b.URL, "/"))
			}
		}
	}
	if len(urls) == 0 {
		return
	}

	for _, u := range urls {
		s.pollBackendMetricsURL(ctx, u)
	}
}

func (s *Server) pollBackendMetricsURL(ctx context.Context, baseURL string) {
	u := baseURL + "/metrics"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return
	}
	metrics := parsePrometheusText(string(body))
	if len(metrics) == 0 {
		return
	}
	now := s.createdAtValue(time.Now().UTC())
	_ = retryDBWrite(func() error {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		for name, value := range metrics {
			if _, err := tx.Exec(s.rebind(`INSERT INTO backend_metrics (created_at, backend_url, metric_name, metric_value) VALUES (?, ?, ?, ?)`), now, baseURL, name, value); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		return tx.Commit()
	})
}

func parsePrometheusText(text string) map[string]float64 {
	out := make(map[string]float64)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		out[name] = value
	}
	return out
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRequest(s scanner, isPostgres bool) (RequestRecord, error) {
	var rec RequestRecord
	var createdAt string
	var query sql.NullString
	var clientIP sql.NullString
	var backendURL sql.NullString
	var model sql.NullString
	var errorText sql.NullString
	var requestRawPath sql.NullString
	var responseRawPath sql.NullString
	var userAgent sql.NullString

	var streamVal any
	if isPostgres {
		var v bool
		streamVal = &v
	} else {
		var v int
		streamVal = &v
	}

	err := s.Scan(
		&rec.ID,
		&createdAt,
		&rec.Method,
		&rec.Path,
		&query,
		&clientIP,
		&backendURL,
		&model,
		streamVal,
		&rec.StatusCode,
		&errorText,
		&rec.RequestBytes,
		&rec.ResponseBytes,
		&rec.PromptTokens,
		&rec.CachedPromptTokens,
		&rec.CacheHitPct,
		&rec.CompletionTokens,
		&rec.TotalTokens,
		&rec.PromptMs,
		&rec.CompletionMs,
		&rec.TotalMs,
		&rec.FirstByteMs,
		&rec.ChunksCount,
		&requestRawPath,
		&responseRawPath,
		&userAgent,
	)
	if err != nil {
		return rec, err
	}
	rec.Query = query.String
	rec.ClientIP = clientIP.String
	rec.BackendURL = backendURL.String
	rec.Model = model.String
	rec.ErrorText = errorText.String
	rec.RequestRawPath = requestRawPath.String
	rec.ResponseRawPath = responseRawPath.String
	rec.UserAgent = userAgent.String
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err == nil {
		rec.CreatedAt = t
	}
	if isPostgres {
		if v, ok := streamVal.(*bool); ok {
			rec.IsStreaming = *v
		}
	} else {
		if v, ok := streamVal.(*int); ok {
			rec.IsStreaming = *v == 1
		}
	}
	if rec.CacheHitPct == 0 && rec.PromptTokens > 0 && rec.CachedPromptTokens > 0 {
		rec.CacheHitPct = float64(rec.CachedPromptTokens) / float64(rec.PromptTokens) * 100
	}
	return rec, nil
}

func enrichRequestRates(rec RequestRecord) RequestRecord {
	if rec.PromptMs > 0 {
		rec.PromptTokPerSec = float64(rec.PromptTokens) / (rec.PromptMs / 1000.0)
	}
	if rec.CompletionMs > 0 {
		rec.DecodeTokPerSec = float64(rec.CompletionTokens) / (rec.CompletionMs / 1000.0)
	}
	if rec.TotalMs > 0 {
		rec.TotalTokPerSec = float64(rec.TotalTokens) / (rec.TotalMs / 1000.0)
	}
	return rec
}
