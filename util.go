package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) rebind(q string) string {
	return s.db.Rebind(q)
}

func (s *Server) isPostgres() bool {
	return s.db.GetType() == "postgresql"
}

func (s *Server) streamValue(v bool) any {
	if s.isPostgres() {
		return v
	}
	return boolToInt(v)
}

func (s *Server) createdAtValue(t time.Time) any {
	if s.isPostgres() {
		return t.UTC()
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (s *Server) isStreamingValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	case []byte:
		return len(x) > 0 && x[0] == '1'
	case string:
		return x == "1" || x == "true" || x == "t"
	default:
		return false
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func copyRequestHeaders(dst http.Header, src http.Header) {
	for k, vals := range src {
		if isHopByHopHeader(k) || strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func isHopByHopHeader(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
func getClientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

var errTooLarge = errors.New("body exceeds max size")

func readWithLimit(rc io.ReadCloser, maxBytes int64) ([]byte, error) {
	defer rc.Close()
	if maxBytes <= 0 {
		return io.ReadAll(rc)
	}
	limited := io.LimitReader(rc, maxBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, errTooLarge
	}
	return b, nil
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case int32:
		return int64(x)
	case json.Number:
		i, _ := x.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(x, 10, 64)
		return i
	default:
		return 0
	}
}

func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func getQueryInt(r *http.Request, name string, fallback int) int {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func getQueryBool(r *http.Request, name string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getOptionalQueryBool(r *http.Request, name string) (bool, bool) {
	v := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(name)))
	if v == "" {
		return false, false
	}
	switch v {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}
func validateBackendURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("backend URL is empty")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return fmt.Errorf("backend URL must start with http:// or https://")
	}
	return nil
}
