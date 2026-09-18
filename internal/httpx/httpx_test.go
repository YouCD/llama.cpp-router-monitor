package httpx

import "testing"

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
		if got := HostOnly(c.in); got != c.want {
			t.Errorf("HostOnly(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
