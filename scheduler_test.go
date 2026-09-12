package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeProbe 返回一个就绪探测函数：前 attempts 次失败，之后成功。
func fakeProbe(attempts int) func(context.Context, string) bool {
	n := 0
	return func(context.Context, string) bool {
		n++
		return n > attempts
	}
}

func newTestScheduler(t *testing.T, idle time.Duration) *Scheduler {
	t.Helper()
	cfg := &SchedulingConfig{
		Coding: CodingConfig{
			Command:      "",
			Header:       map[string]string{"X-LLM-Purpose": "coding"},
			ReadinessURL: "http://127.0.0.1:8080",
		},
		Background: BackgroundConfig{
			Command:      "",
			ReadinessURL: "http://127.0.0.1:8080",
		},
	}
	sw := SwitchConfig{DrainTimeout: time.Millisecond, KillTimeout: time.Millisecond, StartupTimeout: 500 * time.Millisecond}
	s := NewScheduler(cfg, idle, sw, &http.Client{})
	s.SetLogger(func(string, ...any) {})
	s.SetProbe(fakeProbe(0)) // 立即就绪
	return s
}

func TestCodingHeaderMatch(t *testing.T) {
	s := newTestScheduler(t, time.Hour)

	r, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("X-LLM-Purpose", "coding")
	if !s.codingHeaderMatch(r.Header) {
		t.Fatal("expected coding match")
	}

	r2, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	r2.Header.Set("X-LLM-Purpose", "other")
	if s.codingHeaderMatch(r2.Header) {
		t.Fatal("unexpected coding match for other purpose")
	}

	r3, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	if s.codingHeaderMatch(r3.Header) {
		t.Fatal("unexpected coding match without header")
	}
}

func TestStartBackgroundThenEnsureCoding(t *testing.T) {
	s := newTestScheduler(t, time.Hour)
	ctx := context.Background()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.mode != modeBackground || !s.readyOK {
		t.Fatalf("expected background ready, got mode=%s ready=%v", s.mode, s.readyOK)
	}
	if s.IsCodingActive() {
		t.Fatal("should not be coding active initially")
	}

	ready, err := s.EnsureCoding(ctx)
	if err != nil {
		t.Fatalf("ensure coding: %v", err)
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("coding did not become ready")
	}
	if s.mode != modeCoding {
		t.Fatalf("expected coding mode, got %s", s.mode)
	}
	if !s.IsCodingActive() {
		t.Fatal("should be coding active after coding request")
	}
	if got := s.ActiveBaseURL(); got != "http://127.0.0.1:8080/v1" {
		t.Fatalf("unexpected base url: %s", got)
	}
}

func TestEnsureCodingPendingSharesSwitch(t *testing.T) {
	s := newTestScheduler(t, time.Hour)
	ctx := context.Background()
	_ = s.Start(ctx)

	// 让探针暂时失败，使切换进入 pending 状态。
	s.SetProbe(fakeProbe(10))
	c1, _ := s.EnsureCoding(ctx)
	c2, _ := s.EnsureCoding(ctx)
	if c1 != c2 {
		t.Fatal("concurrent EnsureCoding should share the same ready channel")
	}
}

func TestIdleSwitchBackToBackground(t *testing.T) {
	s := newTestScheduler(t, 50*time.Millisecond)
	ctx := context.Background()
	_ = s.Start(ctx)

	ready, _ := s.EnsureCoding(ctx)
	<-ready
	if s.mode != modeCoding {
		t.Fatalf("expected coding mode, got %s", s.mode)
	}

	s.TouchCoding()
	time.Sleep(120 * time.Millisecond)
	s.ensureTarget(ctx, modeBackground, "", "http://127.0.0.1:8080", "", "")
	if s.mode != modeBackground {
		t.Fatalf("expected background after idle, got %s", s.mode)
	}
}

func TestNonCodingRejectedDuringCoding(t *testing.T) {
	s := newTestScheduler(t, time.Hour)
	ctx := context.Background()
	_ = s.Start(ctx)
	ready, _ := s.EnsureCoding(ctx)
	<-ready

	if !s.IsCodingActive() {
		t.Fatal("should be coding active")
	}
	if s.codingHeaderMatch(http.Header{}) {
		t.Fatal("empty header must not be coding")
	}
}

func TestProcessLogFileCaptured(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "coding.log")

	s := newTestScheduler(t, time.Hour)
	s.SetProbe(fakeProbe(0))
	ctx := context.Background()

	// 用真实命令写入 stdout/stderr，验证日志文件接管。
	_, err := s.ensureTarget(ctx, modeCoding, "printf 'hello-stdout\\n' && printf 'hello-stderr\\n' >&2", "http://127.0.0.1:8080", "", logPath)
	if err != nil {
		t.Fatalf("ensureTarget: %v", err)
	}
	// 等待进程输出落盘。
	time.Sleep(200 * time.Millisecond)
	s.stopProcess()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "hello-stdout") || !strings.Contains(out, "hello-stderr") {
		t.Fatalf("log file missing output: %q", out)
	}
}

func TestOpenProcessLogEmpty(t *testing.T) {
	f, err := openProcessLog("")
	if err != nil {
		t.Fatalf("open empty log: %v", err)
	}
	if f != nil {
		t.Fatal("expected nil file for empty log path")
	}
}

func TestReconcileReadyAfterStartupTimeout(t *testing.T) {
	cfg := &SchedulingConfig{
		Coding:     CodingConfig{Command: "sleep 30", Header: map[string]string{"X-LLM-Purpose": "coding"}, ReadinessURL: "http://127.0.0.1:8080"},
		Background: BackgroundConfig{Command: "sleep 30", ReadinessURL: "http://127.0.0.1:8080"},
	}
	sw := SwitchConfig{DrainTimeout: time.Millisecond, KillTimeout: time.Millisecond, StartupTimeout: 300 * time.Millisecond}
	s := NewScheduler(cfg, time.Hour, sw, &http.Client{})
	s.SetLogger(func(string, ...any) {})
	// 先让 probe 一直失败，触发启动超时（readyOK 保持 false）。
	s.SetProbe(func(context.Context, string) bool { return false })
	ctx := context.Background()

	if _, err := s.ensureTarget(ctx, modeCoding, "sleep 30", "http://127.0.0.1:8080", "", ""); err == nil {
		t.Fatal("expected startup timeout error")
	}
	if s.readyOK {
		t.Fatal("expected readyOK=false after startup timeout")
	}
	if s.ActiveBaseURL() != "" {
		t.Fatal("expected empty ActiveBaseURL before ready")
	}

	// 进程实际就绪后，reconcileReady 应恢复 readyOK 并放行等待者。
	s.SetProbe(func(context.Context, string) bool { return true })
	s.reconcileReady()

	if !s.readyOK {
		t.Fatal("expected readyOK=true after reconcile")
	}
	if got := s.ActiveBaseURL(); got != "http://127.0.0.1:8080/v1" {
		t.Fatalf("unexpected ActiveBaseURL: %q", got)
	}
	select {
	case <-s.getReadyCh():
	default:
		t.Fatal("expected readyCh to be closed after reconcile")
	}
	s.Shutdown()
}

func TestWaitReadySendsAuthorization(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &SchedulingConfig{
		Coding:     CodingConfig{Command: "", Header: map[string]string{"X-LLM-Purpose": "coding"}, ReadinessURL: server.URL},
		Background: BackgroundConfig{Command: "", ReadinessURL: server.URL},
	}
	sw := SwitchConfig{DrainTimeout: time.Millisecond, KillTimeout: time.Millisecond, StartupTimeout: time.Second}
	s := NewScheduler(cfg, time.Hour, sw, server.Client())
	s.SetLogger(func(string, ...any) {})

	if err := s.waitReady(context.Background(), server.URL, "Bearer secret-token", time.Second); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("expected Authorization header %q, got %q", "Bearer secret-token", gotAuth)
	}
}

func TestAPIBaseFromReadiness(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080/v1"},
		{"http://127.0.0.1:8080/", "http://127.0.0.1:8080/v1"},
		{"http://127.0.0.1:8080/health", "http://127.0.0.1:8080/v1"},
		{"http://127.0.0.1:8080/v1/models", "http://127.0.0.1:8080/v1"},
		{"https://example.com:8443/v1/chat/completions", "https://example.com:8443/v1"},
		{"", ""},
		{"not a url", ""},
	}
	for _, c := range cases {
		if got := apiBaseFromReadiness(c.in); got != c.want {
			t.Errorf("apiBaseFromReadiness(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
