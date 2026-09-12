package main

import (
	"net/http/httptest"
	"testing"
)

func TestHostOnly(t *testing.T) {
	cases := []struct{ in, want string }{
		{"llm.youcd.online", "llm.youcd.online"},
		{"llm.youcd.online:8082", "llm.youcd.online"},
		{"example.com:443", "example.com"},
		{"127.0.0.1:8000", "127.0.0.1"},
		{"[::1]:8080", "::1"},
		{"  spaced.com  ", "spaced.com"},
		{"", ""},
	}
	for _, c := range cases {
		if got := hostOnly(c.in); got != c.want {
			t.Errorf("hostOnly(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUIHostAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		reqHost string
		wantOK  bool
	}{
		{"empty allowlist allows all", nil, "anything:1234", true},
		{"exact match no port", []string{"llm.youcd.online"}, "llm.youcd.online", true},
		{"match ignoring port in request", []string{"llm.youcd.online"}, "llm.youcd.online:8082", true},
		{"match ignoring port in config", []string{"llm.youcd.online:8082"}, "llm.youcd.online", true},
		{"case insensitive", []string{"LLM.Youcd.Online"}, "llm.youcd.online", true},
		{"unlisted host rejected", []string{"llm.youcd.online"}, "evil.com", false},
		{"ip rejected when not listed", []string{"llm.youcd.online"}, "192.168.1.1:8000", false},
		{"no host rejected when allowlist set", []string{"llm.youcd.online"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Server{}
			svc.cfg.UIAllowedHosts = tt.allowed
			req := httptest.NewRequest("GET", "/_proxy/ui", nil)
			req.Host = tt.reqHost
			if got := svc.uiHostAllowed(req); got != tt.wantOK {
				t.Fatalf("uiHostAllowed(%q) = %v, want %v", tt.reqHost, got, tt.wantOK)
			}
		})
	}
}
