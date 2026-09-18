package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadYAMLConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  listen_addr: ":9999"
  data_dir: "/tmp/monitor"

database:
  type: "postgresql"
  postgresql:
    dsn: "postgres://user:pass@localhost:5432/db"

backends:
  allow_dynamic: false
  strategy: "wrr"
  list:
    - name: "backend-1"
      url: "http://backend-1:8080"
      weight: 50
      enabled: true
    - name: "backend-2"
      url: "http://backend-2:8080"
      weight: 30
      enabled: true
    - name: "backend-3"
      url: "http://backend-3:8080"
      weight: 20
      enabled: true

proxy:
  retention_days: 30
  max_request_bytes: 1048576
  max_capture_bytes: 1048576
  request_timeout_seconds: 300
  poll_backend_metrics: false
  poll_interval_seconds: 30
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load yaml config: %v", err)
	}

	if cfg.Server.ListenAddr != ":9999" {
		t.Fatalf("listen_addr=%q", cfg.Server.ListenAddr)
	}
	if cfg.Database.Type != "postgresql" {
		t.Fatalf("db type=%q", cfg.Database.Type)
	}
	if cfg.Database.PostgreSQL.DSN != "postgres://user:pass@localhost:5432/db" {
		t.Fatalf("dsn=%q", cfg.Database.PostgreSQL.DSN)
	}
	if cfg.Backends.AllowDynamic {
		t.Fatal("allow_dynamic should be false")
	}
	if cfg.Backends.Strategy != "wrr" {
		t.Fatalf("strategy=%q", cfg.Backends.Strategy)
	}
	if len(cfg.Backends.List) != 3 {
		t.Fatalf("backends=%d", len(cfg.Backends.List))
	}
	if cfg.Proxy.RetentionDays != 30 {
		t.Fatalf("retention_days=%d", cfg.Proxy.RetentionDays)
	}
	if cfg.Proxy.PollBackendMetrics != nil && *cfg.Proxy.PollBackendMetrics {
		t.Fatal("poll_backend_metrics should be false")
	}

	legacy := cfg.ToLegacy()
	if legacy.ListenAddr != ":9999" {
		t.Fatalf("listen=%q", legacy.ListenAddr)
	}
	if legacy.RequestTimeout.Seconds() != 300 {
		t.Fatalf("timeout=%v", legacy.RequestTimeout)
	}
}

func TestLoadYAMLConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `server: {}
database: {}
backends: {}
proxy: {}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load yaml config: %v", err)
	}

	if cfg.Server.ListenAddr != ":9091" {
		t.Fatalf("listen_addr=%q", cfg.Server.ListenAddr)
	}
	if cfg.Database.Type != "sqlite" {
		t.Fatalf("db type=%q", cfg.Database.Type)
	}
	if cfg.Backends.Strategy != "wrr" {
		t.Fatalf("strategy=%q", cfg.Backends.Strategy)
	}
	if cfg.Proxy.RetentionDays != 14 {
		t.Fatalf("retention_days=%d", cfg.Proxy.RetentionDays)
	}
	if !(cfg.Proxy.PollBackendMetrics != nil && *cfg.Proxy.PollBackendMetrics) {
		t.Fatalf("expected poll_backend_metrics=true")
	}
	if cfg.Proxy.PollInterval != 10 {
		t.Fatalf("poll_interval=%d", cfg.Proxy.PollInterval)
	}
}

func TestLoadYAMLConfigScheduling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  listen_addr: ":9091"
database:
  type: "sqlite"
  sqlite:
    path: "proxy.db"
backends:
  allow_dynamic: true
  list:
    - name: "ext"
      url: "http://ext:8080"
      enabled: true
proxy: {}
scheduling:
  coding:
    command: "llama-server -m coding.gguf --port 8080"
    header:
      X-LLM-Purpose: coding
    readiness_url: "http://127.0.0.1:8080"
  background:
    command: "llama-server -m bg.gguf --port 8080"
    readiness_url: "http://127.0.0.1:8080"
  lease:
    coding_idle_timeout: 45m
  switch:
    drain_timeout: 15s
    kill_timeout: 20s
    startup_timeout: 200s
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load yaml config: %v", err)
	}

	if !cfg.HasScheduling() {
		t.Fatal("hasScheduling should be true")
	}
	if cfg.Scheduling.Coding.Command != "llama-server -m coding.gguf --port 8080" {
		t.Fatalf("coding command=%q", cfg.Scheduling.Coding.Command)
	}
	if cfg.Scheduling.Coding.Header["X-LLM-Purpose"] != "coding" {
		t.Fatalf("coding header=%v", cfg.Scheduling.Coding.Header)
	}
	if cfg.Scheduling.Background.ReadinessURL != "http://127.0.0.1:8080" {
		t.Fatalf("bg readiness=%q", cfg.Scheduling.Background.ReadinessURL)
	}
	if cfg.Scheduling.Lease.CodingIdleTimeout != 45*time.Minute {
		t.Fatalf("idle timeout=%v", cfg.Scheduling.Lease.CodingIdleTimeout)
	}
	if cfg.Scheduling.Switch.DrainTimeout != 15*time.Second || cfg.Scheduling.Switch.KillTimeout != 20*time.Second || cfg.Scheduling.Switch.StartupTimeout != 200*time.Second {
		t.Fatalf("switch=%+v", cfg.Scheduling.Switch)
	}
}

func TestLoadYAMLConfigNoSchedulingDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "server: {}\ndatabase: {}\nbackends: {}\nproxy: {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load yaml config: %v", err)
	}
	if cfg.HasScheduling() {
		t.Fatal("hasScheduling should be false without scheduling config")
	}
	if cfg.Scheduling != nil {
		t.Fatal("scheduling should be nil without scheduling config")
	}
}

func TestLoadYAMLConfigSchedulingDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server: {}
database: {}
backends: {}
proxy: {}
scheduling:
  coding:
    command: "llama-server -m coding.gguf"
    readiness_url: "http://127.0.0.1:8080"
  background:
    command: "llama-server -m bg.gguf"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load yaml config: %v", err)
	}
	if !cfg.HasScheduling() {
		t.Fatal("hasScheduling should be true")
	}
	if cfg.Scheduling.Coding.Header["X-LLM-Purpose"] != "coding" {
		t.Fatalf("default coding header=%v", cfg.Scheduling.Coding.Header)
	}
	if cfg.Scheduling.Lease.CodingIdleTimeout != 30*time.Minute {
		t.Fatalf("default idle timeout=%v", cfg.Scheduling.Lease.CodingIdleTimeout)
	}
	if cfg.Scheduling.Switch.DrainTimeout != 10*time.Second || cfg.Scheduling.Switch.KillTimeout != 10*time.Second || cfg.Scheduling.Switch.StartupTimeout != 120*time.Second {
		t.Fatalf("default switch=%+v", cfg.Scheduling.Switch)
	}
}

// TestLoadYAMLConfigRouting 验证 routing 规则解析与校验：合法配置正常加载，
// pool 为空或 match 无条件时报错。
func TestLoadYAMLConfigRouting(t *testing.T) {
	dir := t.TempDir()

	writeAndLoad := func(t *testing.T, content string) (*YAMLConfig, error) {
		t.Helper()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return Load(path)
	}

	content := `
server:
  listen_addr: ":9999"
  data_dir: "/tmp/monitor"
backends:
  list:
    - name: "b1"
      url: "http://b1:8080"
      weight: 1
      tool_call: true
      tags: ["fast"]
routing:
  rules:
    - name: "goclaw"
      match:
        user_agent_contains: "GoClaw"
      pool: "tool_call"
    - name: "agent-x"
      match:
        user_agent_contains: "AgentX"
        header:
          X-Agent-Env: "prod"
      pool: "fast"
`
	cfg, err := writeAndLoad(t, content)
	if err != nil {
		t.Fatalf("load yaml config: %v", err)
	}
	if cfg.Routing == nil || len(cfg.Routing.Rules) != 2 {
		t.Fatalf("routing rules=%+v, want 2", cfg.Routing)
	}
	r0 := cfg.Routing.Rules[0]
	if r0.Name != "goclaw" || r0.Pool != "tool_call" || r0.Match.UserAgentContains != "GoClaw" {
		t.Fatalf("rule[0]=%+v", r0)
	}
	r1 := cfg.Routing.Rules[1]
	if r1.Pool != "fast" || r1.Match.Headers["X-Agent-Env"] != "prod" {
		t.Fatalf("rule[1]=%+v", r1)
	}
	// tool_call 与 tags 合并为有效标签集
	tags := cfg.Backends.List[0].EffectiveTags()
	if !tags["tool_call"] || !tags["fast"] {
		t.Fatalf("effective tags=%v, want tool_call and fast", tags)
	}

	// pool 为空
	if _, err := writeAndLoad(t, "routing:\n  rules:\n    - match:\n        user_agent_contains: \"X\"\n"); err == nil {
		t.Fatal("expected error for empty pool")
	}
	// match 无条件
	if _, err := writeAndLoad(t, "routing:\n  rules:\n    - pool: \"fast\"\n"); err == nil {
		t.Fatal("expected error for empty match")
	}
}
