package main

import (
	"net/http"
	"strings"
)

// uiHostAllowed 判断当前请求的 Host 是否允许访问 UI 面板。
// 未配置 ui_allowed_hosts 时放行所有 Host；配置后仅放行列表中的域名（忽略端口与大小写）。
func (s *Server) uiHostAllowed(r *http.Request) bool {
	allowed := s.cfg.UIAllowedHosts
	if len(allowed) == 0 {
		return true
	}
	host := hostOnly(r.Host)
	if host == "" {
		return false
	}
	for _, h := range allowed {
		if strings.EqualFold(host, hostOnly(h)) {
			return true
		}
	}
	return false
}

// hostOnly 从 host[:port] 中提取主机名部分（忽略端口），支持 IPv6 括号形式。
func hostOnly(hostport string) string {
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
