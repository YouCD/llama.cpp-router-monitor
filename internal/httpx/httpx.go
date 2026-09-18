// Package httpx 提供 HTTP 响应写出、请求头处理、限长读取与查询参数等通用工具。
package httpx

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

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func CopyRequestHeaders(dst http.Header, src http.Header) {
	for k, vals := range src {
		if IsHopByHopHeader(k) || strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func IsHopByHopHeader(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func ClientIP(r *http.Request) string {
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

// ErrTooLarge 表示请求体超过 max size 限制。
var ErrTooLarge = errors.New("body exceeds max size")

func ReadWithLimit(rc io.ReadCloser, maxBytes int64) ([]byte, error) {
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
		return nil, ErrTooLarge
	}
	return b, nil
}
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func QueryInt(r *http.Request, name string, fallback int) int {
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

func QueryBool(r *http.Request, name string, fallback bool) bool {
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

func OptionalQueryBool(r *http.Request, name string) (bool, bool) {
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

// HostOnly 从 host[:port] 中提取主机名部分（忽略端口），支持 IPv6 括号形式。
func HostOnly(hostport string) string {
	host := strings.TrimSpace(hostport)
	if host == "" {
		return ""
	}
	// IPv6 字面量：形如 [::1] 或 [::1]:8080，取括号内内容。
	if strings.HasPrefix(host, "[") {
		if end := strings.IndexByte(host, ']'); end >= 0 {
			return host[1:end]
		}
	}
	// 普通 host:port，按最后一个冒号切分。
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		// 排除纯 IPv6 无端口的情况（多个冒号且无端口部分）。
		if strings.Count(host, ":") == 1 {
			host = host[:i]
		}
	}
	return host
}

// ParseTime 解析筛选时间参数，支持 RFC3339（含时区，如 2026-09-13T00:50:59+08:00 / ...Z）
// 及本地时间写法，解析成功时返回 UTC 时间。
func ParseTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
