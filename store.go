package main

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/youcd/toolkit/log"
)

func (s *Server) saveRawPayload(requestID string, kind string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	dateDir := time.Now().UTC().Format("2006-01-02")
	relDir := filepath.Join("raw", dateDir)
	fullDir := filepath.Join(s.cfg.DataDir, relDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		return "", err
	}
	fileName := fmt.Sprintf("%s-%s.gz", requestID, kind)
	relPath := filepath.Join(relDir, fileName)
	fullPath := filepath.Join(s.cfg.DataDir, relPath)

	f, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	if _, err := gw.Write(data); err != nil {
		_ = gw.Close()
		return "", err
	}
	if err := gw.Close(); err != nil {
		return "", err
	}
	return relPath, nil
}

func readGzipFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(gr)
}
func normalizeDB(db Database) error {
	_, err := db.Exec(`UPDATE requests SET
		query = COALESCE(query, ''),
		client_ip = COALESCE(client_ip, ''),
		backend_url = COALESCE(backend_url, ''),
		model = COALESCE(model, ''),
		error_text = COALESCE(error_text, ''),
		request_raw_path = COALESCE(request_raw_path, ''),
		response_raw_path = COALESCE(response_raw_path, ''),
		user_agent = COALESCE(user_agent, '')
		WHERE
			query IS NULL OR
			client_ip IS NULL OR
			backend_url IS NULL OR
			model IS NULL OR
			error_text IS NULL OR
			request_raw_path IS NULL OR
			response_raw_path IS NULL OR
			user_agent IS NULL`)
	return err
}

func retryDBWrite(op func() error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = op(); err == nil {
			return nil
		}
		if !isBusyError(err) {
			return err
		}
		time.Sleep(time.Duration(40*(attempt+1)) * time.Millisecond)
	}
	return err
}

func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, pat := range []string{
		"database is locked",
		"sqlbusy",
		"sqlite_busy",
		"busy",
		"deadlock detected",
		"deadlock_detected",
		"could not serialize access",
		"serialization_failure",
		"lock timeout",
		"lock_timeout",
		"database table is locked",
	} {
		if strings.Contains(msg, pat) {
			return true
		}
	}
	return false
}

func findRawPayloadPath(dataDir, requestID, kind string) (string, bool) {
	rawRoot := filepath.Join(dataDir, "raw")
	entries, err := os.ReadDir(rawRoot)
	if err != nil {
		return "", false
	}
	fileName := fmt.Sprintf("%s-%s.gz", requestID, kind)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(rawRoot, entry.Name(), fileName)
		if _, err := os.Stat(fullPath); err == nil {
			return filepath.Join("raw", entry.Name(), fileName), true
		}
	}
	return "", false
}
func repairStuckRequests(db Database, dataDir string) error {
	rows, err := db.Query(`SELECT
		id, created_at, method, path, query, client_ip, backend_url, model,
		is_streaming, status_code, error_text, request_bytes, response_bytes,
		prompt_tokens, cached_prompt_tokens, cache_hit_pct, completion_tokens, total_tokens,
		prompt_ms, completion_ms, total_ms, first_byte_ms, chunks_count,
		request_raw_path, response_raw_path, user_agent
		FROM requests WHERE status_code = 0 AND (response_raw_path IS NULL OR response_raw_path = '')`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		rec, err := scanRequest(rows, db.GetType() == "postgresql")
		if err != nil {
			return err
		}
		respRel, ok := findRawPayloadPath(dataDir, rec.ID, "response")
		if !ok {
			continue
		}
		respAbs := filepath.Join(dataDir, respRel)
		respBytes, err := readGzipFile(respAbs)
		if err != nil {
			continue
		}
		meta := parseResponseMeta(http.Header{"Content-Type": []string{"text/event-stream"}}, respBytes)
		if meta.Model != "" {
			rec.Model = meta.Model
		}
		rec.StatusCode = http.StatusOK
		rec.ResponseBytes = int64(len(respBytes))
		rec.PromptTokens = meta.PromptTokens
		rec.CachedPromptTokens = meta.CachedPromptTokens
		if meta.PromptTokens > 0 && meta.CachedPromptTokens > 0 {
			rec.CacheHitPct = float64(meta.CachedPromptTokens) / float64(meta.PromptTokens) * 100
		}
		rec.CompletionTokens = meta.CompletionTok
		rec.TotalTokens = meta.TotalTokens
		rec.PromptMs = meta.PromptMs
		rec.CompletionMs = meta.CompletionMs
		if stat, statErr := os.Stat(respAbs); statErr == nil {
			rec.TotalMs = float64(stat.ModTime().UTC().Sub(rec.CreatedAt.UTC()).Milliseconds())
		}
		rec.ResponseRawPath = respRel
		if err := retryDBWrite(func() error {
			_, err := db.Exec(db.Rebind(`UPDATE requests SET
				model = ?,
				status_code = ?,
				error_text = ?,
				response_bytes = ?,
				prompt_tokens = ?,
				cached_prompt_tokens = ?,
				cache_hit_pct = ?,
				completion_tokens = ?,
				total_tokens = ?,
				prompt_ms = ?,
				completion_ms = ?,
				total_ms = ?,
				first_byte_ms = ?,
				chunks_count = ?,
				response_raw_path = ?
				WHERE id = ?`),
				rec.Model, rec.StatusCode, rec.ErrorText, rec.ResponseBytes,
				rec.PromptTokens, rec.CachedPromptTokens, rec.CacheHitPct, rec.CompletionTokens, rec.TotalTokens,
				rec.PromptMs, rec.CompletionMs, rec.TotalMs, rec.FirstByteMs,
				rec.ChunksCount, rec.ResponseRawPath, rec.ID,
			)
			return err
		}); err != nil {
			continue
		}
	}
	return rows.Err()
}
func (s *Server) insertRequest(rec RequestRecord) error {
	return retryDBWrite(func() error {
		_, err := s.db.Exec(s.rebind(`INSERT INTO requests (
			id, created_at, method, path, query, client_ip, backend_url, model,
			is_streaming, request_bytes, request_raw_path, user_agent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			rec.ID, s.createdAtValue(rec.CreatedAt), rec.Method, rec.Path, rec.Query,
			rec.ClientIP, rec.BackendURL, rec.Model, s.streamValue(rec.IsStreaming),
			rec.RequestBytes, rec.RequestRawPath, rec.UserAgent,
		)
		return err
	})
}

func (s *Server) finishRequest(id string, rec RequestRecord) error {
	return retryDBWrite(func() error {
		_, err := s.db.Exec(s.rebind(`UPDATE requests SET
			model = ?,
			status_code = ?,
			error_text = ?,
			response_bytes = ?,
			prompt_tokens = ?,
			cached_prompt_tokens = ?,
			cache_hit_pct = ?,
			completion_tokens = ?,
			total_tokens = ?,
			prompt_ms = ?,
			completion_ms = ?,
			total_ms = ?,
			first_byte_ms = ?,
			chunks_count = ?,
			response_raw_path = ?
			WHERE id = ?`),
			rec.Model, rec.StatusCode, rec.ErrorText, rec.ResponseBytes,
			rec.PromptTokens, rec.CachedPromptTokens, rec.CacheHitPct, rec.CompletionTokens, rec.TotalTokens,
			rec.PromptMs, rec.CompletionMs, rec.TotalMs, rec.FirstByteMs,
			rec.ChunksCount, rec.ResponseRawPath, id,
		)
		return err
	})
}

func (s *Server) getRequests(limit int, offset int, f RequestFilter) ([]RequestRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	query := `SELECT
		id, created_at, method, path, query, client_ip, backend_url, model,
		is_streaming, status_code, error_text, request_bytes, response_bytes,
		prompt_tokens, cached_prompt_tokens, cache_hit_pct, completion_tokens, total_tokens,
		prompt_ms, completion_ms, total_ms, first_byte_ms, chunks_count,
		request_raw_path, response_raw_path, user_agent
		FROM requests WHERE 1=1`
	args := make([]any, 0, 16)

	query, args = appendRequestFilterSQL(query, args, f, true, s.isPostgres())

	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(s.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RequestRecord, 0, limit)
	for rows.Next() {
		rec, err := scanRequest(rows, s.isPostgres())
		if err != nil {
			return nil, err
		}
		rec = enrichRequestRates(rec)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Server) getRequestByID(id string) (RequestRecord, error) {
	row := s.db.QueryRow(s.rebind(`SELECT
		id, created_at, method, path, query, client_ip, backend_url, model,
		is_streaming, status_code, error_text, request_bytes, response_bytes,
		prompt_tokens, cached_prompt_tokens, cache_hit_pct, completion_tokens, total_tokens,
		prompt_ms, completion_ms, total_ms, first_byte_ms, chunks_count,
		request_raw_path, response_raw_path, user_agent
		FROM requests WHERE id = ?`), id)
	rec, err := scanRequest(row, s.isPostgres())
	if err != nil {
		return rec, err
	}
	return enrichRequestRates(rec), nil
}

func (s *Server) deleteRequestByID(id string) error {
	rec, err := s.getRequestByID(id)
	if err != nil {
		return err
	}
	for _, rel := range []string{rec.RequestRawPath, rec.ResponseRawPath} {
		if rel == "" {
			continue
		}
		fullPath := filepath.Clean(filepath.Join(s.cfg.DataDir, rel))
		relCheck, relErr := filepath.Rel(s.cfg.DataDir, fullPath)
		if relErr != nil || strings.HasPrefix(relCheck, "..") {
			return fmt.Errorf("refusing to delete path outside data dir")
		}
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	var affected int64
	err = retryDBWrite(func() error {
		res, err := s.db.Exec(s.rebind(`DELETE FROM requests WHERE id = ?`), id)
		if err != nil {
			return err
		}
		affected, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *Server) getStats(hours int, f RequestFilter) (map[string]any, error) {
	if hours <= 0 {
		hours = 24
	}
	if f.SinceHours > 0 {
		hours = f.SinceHours
	}

	var streamSumSQL = `COALESCE(SUM(is_streaming),0)`
	if s.isPostgres() {
		streamSumSQL = `COALESCE(SUM(CASE WHEN is_streaming THEN 1 ELSE 0 END),0)`
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	sinceVal := s.createdAtValue(since)

	var totalRequests int64
	var promptTokens int64
	var completionTokens int64
	var totalTokens int64
	var lifetimeRequests int64
	var lifetimeTokens int64
	var matchingRequests int64
	var matchingTokens int64
	var reqBytes int64
	var respBytes int64
	var avgPromptMs float64
	var avgCompletionMs float64
	var avgTotalMs float64
	var avgFirstByteMs float64
	var errorsCount int64
	var streamCount int64

	query := `SELECT
		COUNT(*),
		COALESCE(SUM(prompt_tokens),0),
		COALESCE(SUM(completion_tokens),0),
		COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(request_bytes),0),
		COALESCE(SUM(response_bytes),0),
		COALESCE(AVG(prompt_ms),0),
		COALESCE(AVG(completion_ms),0),
		COALESCE(AVG(total_ms),0),
		COALESCE(AVG(first_byte_ms),0),
		COALESCE(SUM(CASE WHEN status_code >= 400 OR error_text != '' THEN 1 ELSE 0 END),0),
		` + streamSumSQL + `
		FROM requests WHERE created_at >= ?`
	args := []any{sinceVal}
	query, args = appendRequestFilterSQL(query, args, f, false, s.isPostgres())

	err := s.db.QueryRow(s.rebind(query), args...).Scan(
		&totalRequests,
		&promptTokens,
		&completionTokens,
		&totalTokens,
		&reqBytes,
		&respBytes,
		&avgPromptMs,
		&avgCompletionMs,
		&avgTotalMs,
		&avgFirstByteMs,
		&errorsCount,
		&streamCount,
	)
	if err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(total_tokens),0)
		FROM requests`).Scan(&lifetimeRequests, &lifetimeTokens); err != nil {
		return nil, err
	}
	matchQuery := `SELECT
		COUNT(*),
		COALESCE(SUM(total_tokens),0)
		FROM requests WHERE 1=1`
	matchArgs := []any{}
	matchQuery, matchArgs = appendRequestFilterSQL(matchQuery, matchArgs, f, true, s.isPostgres())
	if err := s.db.QueryRow(s.rebind(matchQuery), matchArgs...).Scan(&matchingRequests, &matchingTokens); err != nil {
		return nil, err
	}

	secs := float64(hours * 3600)
	rpm := float64(totalRequests) / float64(hours*60)
	promptTokensPerSec := float64(promptTokens) / secs
	decodeTokensPerSec := float64(completionTokens) / secs
	tokensPerSec := float64(totalTokens) / secs
	errorRate := 0.0
	if totalRequests > 0 {
		errorRate = float64(errorsCount) / float64(totalRequests)
	}

	return map[string]any{
		"hours":                    hours,
		"active_connections":       s.active.Load(),
		"total_requests":           totalRequests,
		"total_prompt_tokens":      promptTokens,
		"total_completion_tokens":  completionTokens,
		"total_tokens":             totalTokens,
		"matching_total_requests":  matchingRequests,
		"matching_total_tokens":    matchingTokens,
		"lifetime_total_requests":  lifetimeRequests,
		"lifetime_total_tokens":    lifetimeTokens,
		"total_request_bytes":      reqBytes,
		"total_response_bytes":     respBytes,
		"avg_prompt_ms":            avgPromptMs,
		"avg_completion_ms":        avgCompletionMs,
		"avg_total_ms":             avgTotalMs,
		"avg_first_byte_ms":        avgFirstByteMs,
		"requests_per_minute":      rpm,
		"prompt_tokens_per_second": promptTokensPerSec,
		"decode_tokens_per_second": decodeTokensPerSec,
		"tokens_per_second":        tokensPerSec,
		"errors_count":             errorsCount,
		"error_rate":               errorRate,
		"streaming_requests":       streamCount,
	}, nil
}

func (s *Server) getModels() ([]string, error) {
	orderSQL := `ORDER BY model COLLATE NOCASE ASC`
	if s.isPostgres() {
		orderSQL = `ORDER BY lower(model) ASC`
	}
	rows, err := s.db.Query(`SELECT DISTINCT model
		FROM requests
		WHERE model IS NOT NULL AND TRIM(model) != ''
		` + orderSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0, 64)
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		items = append(items, model)
	}
	return items, rows.Err()
}
func (s *Server) getBackends() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT backend_url
		FROM requests
		WHERE backend_url IS NOT NULL AND TRIM(backend_url) != ''
		ORDER BY backend_url ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0, 16)
	for rows.Next() {
		var backend string
		if err := rows.Scan(&backend); err != nil {
			return nil, err
		}
		backend = strings.TrimSpace(backend)
		if backend == "" {
			continue
		}
		items = append(items, backend)
	}
	return items, rows.Err()
}

func (s *Server) getStatsByBackend(hours int) ([]map[string]any, error) {
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	sinceVal := s.createdAtValue(since)

	var streamSumSQL = `COALESCE(SUM(is_streaming),0)`
	if s.isPostgres() {
		streamSumSQL = `COALESCE(SUM(CASE WHEN is_streaming THEN 1 ELSE 0 END),0)`
	}

	query := `SELECT
		backend_url,
		COUNT(*),
		COALESCE(SUM(prompt_tokens),0),
		COALESCE(SUM(completion_tokens),0),
		COALESCE(SUM(total_tokens),0),
		COALESCE(AVG(total_ms),0),
		COALESCE(AVG(first_byte_ms),0),
		COALESCE(SUM(CASE WHEN status_code >= 400 OR error_text != '' THEN 1 ELSE 0 END),0),
		` + streamSumSQL + `
		FROM requests
		WHERE created_at >= ? AND backend_url IS NOT NULL AND TRIM(backend_url) != ''
		GROUP BY backend_url
		ORDER BY COUNT(*) DESC`

	rows, err := s.db.Query(s.rebind(query), sinceVal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0, 16)
	for rows.Next() {
		var backend string
		var count int64
		var promptTokens int64
		var completionTokens int64
		var totalTokens int64
		var avgTotalMs float64
		var avgFirstByteMs float64
		var errorsCount int64
		var streamCount int64
		if err := rows.Scan(&backend, &count, &promptTokens, &completionTokens, &totalTokens,
			&avgTotalMs, &avgFirstByteMs, &errorsCount, &streamCount); err != nil {
			return nil, err
		}
		errorRate := 0.0
		if count > 0 {
			errorRate = float64(errorsCount) / float64(count)
		}
		out = append(out, map[string]any{
			"backend_url":        backend,
			"requests":           count,
			"prompt_tokens":      promptTokens,
			"completion_tokens":  completionTokens,
			"total_tokens":       totalTokens,
			"avg_total_ms":       avgTotalMs,
			"avg_first_byte_ms":  avgFirstByteMs,
			"errors_count":       errorsCount,
			"error_rate":         errorRate,
			"streaming_requests": streamCount,
		})
	}
	return out, rows.Err()
}

func (s *Server) getBackendMetrics(limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	rows, err := s.db.Query(s.rebind(`SELECT created_at, backend_url, metric_name, metric_value
		FROM backend_metrics ORDER BY created_at DESC, id DESC LIMIT ?`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var createdAt string
		var backendURL string
		var metricName string
		var metricValue float64
		if err := rows.Scan(&createdAt, &backendURL, &metricName, &metricValue); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"created_at":   createdAt,
			"backend_url":  backendURL,
			"metric_name":  metricName,
			"metric_value": metricValue,
		})
	}
	return out, rows.Err()
}
func (s *Server) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	s.cleanup()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *Server) cleanup() {
	if s.cfg.RetentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -s.cfg.RetentionDays)
	cutoffVal := s.createdAtValue(cutoff)
	if err := retryDBWrite(func() error {
		_, err := s.db.Exec(s.rebind(`DELETE FROM requests WHERE created_at < ?`), cutoffVal)
		return err
	}); err != nil {
		log.WithCtx(context.Background()).Infof("cleanup requests failed: %v", err)
	}
	if err := retryDBWrite(func() error {
		_, err := s.db.Exec(s.rebind(`DELETE FROM backend_metrics WHERE created_at < ?`), cutoffVal)
		return err
	}); err != nil {
		log.WithCtx(context.Background()).Infof("cleanup backend_metrics failed: %v", err)
	}

	rawRoot := filepath.Join(s.cfg.DataDir, "raw")
	entries, err := os.ReadDir(rawRoot)
	if err != nil && !os.IsNotExist(err) {
		log.WithCtx(context.Background()).Infof("cleanup raw read failed: %v", err)
		return
	}
	cutoffDate := time.Now().UTC().AddDate(0, 0, -s.cfg.RetentionDays)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		d, err := time.Parse("2006-01-02", entry.Name())
		if err != nil {
			continue
		}
		if d.Before(cutoffDate) {
			_ = os.RemoveAll(filepath.Join(rawRoot, entry.Name()))
		}
	}
}

func appendRequestFilterSQL(query string, args []any, f RequestFilter, includeSince bool, isPostgres bool) (string, []any) {
	if f.ChatCompletionsOnly {
		query += ` AND method = ? AND path = ?`
		args = append(args, http.MethodPost, "/v1/chat/completions")
	}
	if f.Path != "" {
		query += ` AND path LIKE ?`
		args = append(args, "%"+f.Path+"%")
	}
	if f.Model != "" {
		query += ` AND model LIKE ?`
		args = append(args, "%"+f.Model+"%")
	}
	if f.Method != "" {
		query += ` AND method = ?`
		args = append(args, f.Method)
	}
	if f.Backend != "" {
		query += ` AND backend_url = ?`
		args = append(args, f.Backend)
	}
	if f.StatusCode > 0 {
		query += ` AND status_code = ?`
		args = append(args, f.StatusCode)
	}
	if includeSince && f.SinceHours > 0 {
		since := time.Now().UTC().Add(-time.Duration(f.SinceHours) * time.Hour)
		if isPostgres {
			query += ` AND created_at >= ?`
			args = append(args, since)
		} else {
			query += ` AND created_at >= ?`
			args = append(args, since.Format(time.RFC3339Nano))
		}
	}
	if f.Streaming != nil {
		query += ` AND is_streaming = ?`
		if isPostgres {
			args = append(args, *f.Streaming)
		} else {
			args = append(args, boolToInt(*f.Streaming))
		}
	}
	if f.ErrorsOnly {
		query += ` AND (status_code >= 400 OR error_text != '')`
	}
	if f.WithTokens {
		query += ` AND total_tokens > 0`
	}
	if f.Search != "" {
		query += ` AND (
			id LIKE ? OR path LIKE ? OR query LIKE ? OR client_ip LIKE ? OR model LIKE ? OR error_text LIKE ?
		)`
		pattern := "%" + f.Search + "%"
		args = append(args, pattern, pattern, pattern, pattern, pattern, pattern)
	}
	return query, args
}
