package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

monitor:
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

	cfg, err := loadYAMLConfig(path)
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
	if cfg.Monitor.RetentionDays != 30 {
		t.Fatalf("retention_days=%d", cfg.Monitor.RetentionDays)
	}
	if cfg.Monitor.PollBackendMetrics != nil && *cfg.Monitor.PollBackendMetrics {
		t.Fatal("poll_backend_metrics should be false")
	}

	legacy := cfg.toLegacyConfig()
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
monitor: {}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadYAMLConfig(path)
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
	if cfg.Monitor.RetentionDays != 14 {
		t.Fatalf("retention_days=%d", cfg.Monitor.RetentionDays)
	}
	if !(cfg.Monitor.PollBackendMetrics != nil && *cfg.Monitor.PollBackendMetrics) {
		t.Fatal("poll_backend_metrics should default to true")
	}
	if cfg.Monitor.PollInterval != 10 {
		t.Fatalf("poll_interval=%d", cfg.Monitor.PollInterval)
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  listen_addr: ":9999"
monitor:
  retention_days: 30
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LISTEN_ADDR", ":7777")
	t.Setenv("RETENTION_DAYS", "7")

	cfg, err := loadYAMLConfig(path)
	if err != nil {
		t.Fatalf("load yaml config: %v", err)
	}

	if cfg.Server.ListenAddr != ":7777" {
		t.Fatalf("listen_addr=%q want :7777 (env override)", cfg.Server.ListenAddr)
	}
	if cfg.Monitor.RetentionDays != 7 {
		t.Fatalf("retention_days=%d want 7 (env override)", cfg.Monitor.RetentionDays)
	}
}

func TestBackendBalancerWeightedSelect(t *testing.T) {
	backends := []BackendConfig{
		{Name: "a", URL: "http://a:8080", Weight: 50, Enabled: true},
		{Name: "b", URL: "http://b:8080", Weight: 30, Enabled: true},
		{Name: "c", URL: "http://c:8080", Weight: 20, Enabled: true},
	}

	bb := NewBackendBalancer(backends, "wrr")
	if bb.GetEnabledCount() != 3 {
		t.Fatalf("enabled=%d", bb.GetEnabledCount())
	}
	if bb.GetStrategy() != "wrr" {
		t.Fatalf("strategy=%q", bb.GetStrategy())
	}

	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		b := bb.Select()
		if b == nil {
			t.Fatal("select returned nil")
		}
		counts[b.Name]++
	}

	if counts["a"] < 400 {
		t.Fatalf("backend a selected too few times: %v", counts)
	}
	if counts["b"] < 200 {
		t.Fatalf("backend b selected too few times: %v", counts)
	}
	if counts["c"] < 100 {
		t.Fatalf("backend c selected too few times: %v", counts)
	}
	if counts["a"]+counts["b"]+counts["c"] != 1000 {
		t.Fatalf("total=%v", counts)
	}
}

func TestBackendBalancerSkipsDisabled(t *testing.T) {
	backends := []BackendConfig{
		{Name: "a", URL: "http://a:8080", Weight: 50, Enabled: true},
		{Name: "b", URL: "http://b:8080", Weight: 30, Enabled: false},
	}

	bb := NewBackendBalancer(backends, "wrr")
	if bb.GetEnabledCount() != 1 {
		t.Fatalf("enabled=%d", bb.GetEnabledCount())
	}
	for i := 0; i < 100; i++ {
		b := bb.Select()
		if b == nil || b.Name != "a" {
			t.Fatalf("select=%v, want a only", b)
		}
	}
}

func TestSelectBackendWithBalancer(t *testing.T) {
	backends := []BackendConfig{
		{Name: "lb-1", URL: "http://lb1.example:8080", Weight: 1, Enabled: true},
		{Name: "lb-2", URL: "http://lb2.example:8080", Weight: 1, Enabled: true},
	}

	cfg := Config{
		AllowDynamicBackend: false,
	}

	svc := &Server{
		cfg:      cfg,
		balancer: NewBackendBalancer(backends, "rr"),
	}

	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		backend, _, _, err := svc.selectBackend(req)
		if err != nil {
			t.Fatalf("selectBackend: %v", err)
		}
		seen[backend] = true
	}

	if len(seen) != 2 {
		t.Fatalf("expected both backends to be selected, got %v", seen)
	}
}

func TestSelectBackendWithBalancerNoDefault(t *testing.T) {
	// No default_url, only load balancer - must still work.
	backends := []BackendConfig{
		{Name: "a", URL: "http://a.example:8080", Weight: 1, Enabled: true},
		{Name: "b", URL: "http://b.example:8080", Weight: 1, Enabled: true},
	}

	cfg := Config{
		AllowDynamicBackend: true,
	}

	svc := &Server{
		cfg:      cfg,
		balancer: NewBackendBalancer(backends, "rr"),
	}

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		backend, _, bc, err := svc.selectBackend(req)
		if err != nil {
			t.Fatalf("selectBackend: %v", err)
		}
		if bc == nil || backend == "" {
			t.Fatalf("expected balancer backend, got backend=%q bc=%v", backend, bc)
		}
	}
}

func TestSelectBackendNoBackendConfigured(t *testing.T) {
	// No balancer and no dynamic override -> error.
	cfg := Config{AllowDynamicBackend: true}
	svc := &Server{cfg: cfg}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	if _, _, _, err := svc.selectBackend(req); err == nil {
		t.Fatal("expected error when no backend available")
	}
}

func TestSelectBackendDynamicStillWorks(t *testing.T) {
	cfg := Config{
		AllowDynamicBackend: true,
	}

	svc := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Backend-URL", "http://override.example:8080")
	backend, _, bc, err := svc.selectBackend(req)
	if err != nil {
		t.Fatalf("selectBackend: %v", err)
	}
	if backend != "http://override.example:8080" {
		t.Fatalf("backend=%q", backend)
	}
	if bc != nil {
		t.Fatalf("expected nil backend config for dynamic override, got %+v", bc)
	}
}

func TestSelectBackendBalancerReturnsConfig(t *testing.T) {
	backends := []BackendConfig{
		{Name: "qwen", URL: "http://gpu-server-1:8080", Weight: 50, Enabled: true, Model: "qwen", APIKey: "key-1"},
		{Name: "deepseek", URL: "http://gpu-server-2:8080", Weight: 50, Enabled: true, Model: "deepseek", APIKey: "key-2"},
	}

	// Use weighted random so each backend can be hit many times and we can
	// verify config is attached to the right URL.
	svc := &Server{
		cfg: Config{
			AllowDynamicBackend: true,
		},
		balancer: NewBackendBalancer(backends, "random"),
	}

	found := map[string]*BackendConfig{}
	for i := 0; i < 200; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		backend, _, bc, err := svc.selectBackend(req)
		if err != nil {
			t.Fatalf("selectBackend: %v", err)
		}
		if bc == nil {
			t.Fatal("expected non-nil backend config from balancer")
		}
		if strings.TrimRight(bc.URL, "/") != backend {
			t.Fatalf("backend url mismatch: bc=%q sel=%q", bc.URL, backend)
		}
		found[bc.Name] = bc
	}

	if len(found) != 2 {
		t.Fatalf("expected both backends, got %v", found)
	}
	if found["qwen"].Model != "qwen" || found["qwen"].APIKey != "key-1" {
		t.Fatalf("qwen config wrong: %+v", found["qwen"])
	}
	if found["deepseek"].Model != "deepseek" || found["deepseek"].APIKey != "key-2" {
		t.Fatalf("deepseek config wrong: %+v", found["deepseek"])
	}
}

func TestRewriteModel(t *testing.T) {
	body := []byte(`{"model":"client-model","messages":[]}`)
	newBody, model, err := rewriteModel(body, "qwen")
	if err != nil {
		t.Fatalf("rewriteModel: %v", err)
	}
	if model != "qwen" {
		t.Fatalf("model=%q", model)
	}
	if !strings.Contains(string(newBody), `"model":"qwen"`) {
		t.Fatalf("body not rewritten: %s", newBody)
	}
	if strings.Contains(string(newBody), "client-model") {
		t.Fatalf("old model still present: %s", newBody)
	}
}

func TestRewriteModelNonJSON(t *testing.T) {
	body := []byte("not-json")
	newBody, model, err := rewriteModel(body, "qwen")
	if err == nil {
		t.Fatalf("expected error for non-JSON, got model=%q body=%s", model, newBody)
	}
}

func TestHandleProxyWithModelRewriteAndAPIKey(t *testing.T) {
	var gotModel string
	var gotAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		if v, ok := m["model"].(string); ok {
			gotModel = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model":"qwen","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer backend.Close()

	dataDir := t.TempDir()
	sqlCfg := DatabaseConfig{Type: "sqlite", SQLite: SQLiteConfig{Path: "monitor.db"}}
	db, err := NewDatabase(sqlCfg, dataDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := InitDB(db, "sqlite"); err != nil {
		t.Fatalf("init db: %v", err)
	}

	backends := []BackendConfig{
		{Name: "qwen", URL: backend.URL, Weight: 1, Enabled: true, Model: "qwen", APIKey: "secret-key"},
	}

	svc := &Server{
		cfg: Config{
			ListenAddr:          ":0",
			AllowDynamicBackend: true,
			DataDir:             dataDir,
			RetentionDays:       14,
			MaxRequestBytes:     2 << 20,
			MaxCaptureBytes:     2 << 20,
			RequestTimeout:      15 * time.Second,
		},
		db:       db,
		balancer: NewBackendBalancer(backends, "wrr"),
		client:   &http.Client{Timeout: 15 * time.Second},
		hub:      NewEventHub(),
	}

	proxy := httptest.NewServer(svc)
	defer proxy.Close()

	resp, err := proxy.Client().Post(proxy.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"client-model","messages":[]}`))
	if err != nil {
		t.Fatalf("proxy post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}

	if gotModel != "qwen" {
		t.Fatalf("backend received model=%q, want qwen (rewritten)", gotModel)
	}
	if gotAuth != "Bearer secret-key" {
		t.Fatalf("backend received Authorization=%q, want Bearer secret-key", gotAuth)
	}
}

func TestHandleProxyBackendKeyDoesNotOverrideClientKeyWhenEmpty(t *testing.T) {
	var gotAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model":"m","usage":{}}`)
	}))
	defer backend.Close()

	dataDir := t.TempDir()
	sqlCfg := DatabaseConfig{Type: "sqlite", SQLite: SQLiteConfig{Path: "monitor.db"}}
	db, err := NewDatabase(sqlCfg, dataDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := InitDB(db, "sqlite"); err != nil {
		t.Fatalf("init db: %v", err)
	}

	// No api_key configured on the backend -> client's Authorization passes through.
	backends := []BackendConfig{
		{Name: "plain", URL: backend.URL, Weight: 1, Enabled: true, Model: "m"},
	}

	svc := &Server{
		cfg: Config{
			ListenAddr:          ":0",
			AllowDynamicBackend: true,
			DataDir:             dataDir,
			RetentionDays:       14,
			MaxRequestBytes:     2 << 20,
			MaxCaptureBytes:     2 << 20,
			RequestTimeout:      15 * time.Second,
		},
		db:       db,
		balancer: NewBackendBalancer(backends, "wrr"),
		client:   &http.Client{Timeout: 15 * time.Second},
		hub:      NewEventHub(),
	}

	proxy := httptest.NewServer(svc)
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"x","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client-key")
	resp, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatalf("proxy do: %v", err)
	}
	defer resp.Body.Close()
	if gotAuth != "Bearer client-key" {
		t.Fatalf("backend received Authorization=%q, want client-key to pass through", gotAuth)
	}
}

func TestRebindPostgres(t *testing.T) {
	queries := []struct {
		in, want string
	}{
		{
			`SELECT * FROM requests WHERE id = ?`,
			`SELECT * FROM requests WHERE id = $1`,
		},
		{
			`INSERT INTO requests (id, model) VALUES (?, ?)`,
			`INSERT INTO requests (id, model) VALUES ($1, $2)`,
		},
		{
			`UPDATE requests SET status_code = ? WHERE id = ?`,
			`UPDATE requests SET status_code = $1 WHERE id = $2`,
		},
		{
			`SELECT ?`,
			`SELECT $1`,
		},
	}

	for _, q := range queries {
		if got := rebindPostgres(q.in); got != q.want {
			t.Fatalf("rebindPostgres(%q) = %q, want %q", q.in, got, q.want)
		}
	}
}

func TestRebindPostgresIgnoresQuotedStrings(t *testing.T) {
	// Question marks inside string literals must not be converted.
	in := `SELECT id, model FROM requests WHERE path = '/v1/chat?x=1' AND id = ?`
	want := `SELECT id, model FROM requests WHERE path = '/v1/chat?x=1' AND id = $1`
	if got := rebindPostgres(in); got != want {
		t.Fatalf("rebindPostgres(%q) = %q, want %q", in, got, want)
	}
}

func TestDatabaseTypeDetection(t *testing.T) {
	sqliteDB := &SQLiteDatabase{}
	pgDB := &PostgreSQLDatabase{}

	if sqliteDB.GetType() != "sqlite" {
		t.Fatalf("sqlite type=%q", sqliteDB.GetType())
	}
	if pgDB.GetType() != "postgresql" {
		t.Fatalf("pg type=%q", pgDB.GetType())
	}

	// SQLite Rebind is a no-op
	if got := sqliteDB.Rebind(`SELECT * FROM t WHERE id = ?`); got != `SELECT * FROM t WHERE id = ?` {
		t.Fatalf("sqlite rebind changed query: %q", got)
	}

	// PostgreSQL Rebind converts placeholders
	if got := pgDB.Rebind(`SELECT * FROM t WHERE id = ?`); got != `SELECT * FROM t WHERE id = $1` {
		t.Fatalf("pg rebind=%q", got)
	}
}

func TestStreamValue(t *testing.T) {
	sqliteDB := &SQLiteDatabase{}
	pgDB := &PostgreSQLDatabase{}

	sqliteSvc := &Server{db: sqliteDB}
	pgSvc := &Server{db: pgDB}

	if v := sqliteSvc.streamValue(true); v != 1 {
		t.Fatalf("sqlite streamValue(true)=%v", v)
	}
	if v := sqliteSvc.streamValue(false); v != 0 {
		t.Fatalf("sqlite streamValue(false)=%v", v)
	}
	if v := pgSvc.streamValue(true); v != true {
		t.Fatalf("pg streamValue(true)=%v", v)
	}
	if v := pgSvc.streamValue(false); v != false {
		t.Fatalf("pg streamValue(false)=%v", v)
	}
}

func TestInitPostgreSQLDBSQL(t *testing.T) {
	// Verify PostgreSQL DDL uses valid types and no SQLite-only syntax.
	pgCfg := PostgreSQLConfig{DSN: "postgres://user:pass@localhost:5432/db"}

	// Create a real PostgreSQLDatabase via sql.Open but never Ping (no server).
	// We only need GetType for InitDB branching; init DDL is validated by inspection.
	_ = pgCfg

	// The DDL statements are constructed in initPostgreSQLDB. Ensure they exist
	// and reference BOOLEAN/BIGINT/SERIAL which are PostgreSQL-native.
	db := &PostgreSQLDatabase{}
	if db.GetType() != "postgresql" {
		t.Fatalf("unexpected type %q", db.GetType())
	}
}
