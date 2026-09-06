package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type YAMLConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Backends BackendsConfig `yaml:"backends"`
	Monitor  MonitorConfig  `yaml:"monitor"`
}

type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	DataDir    string `yaml:"data_dir"`
}

type DatabaseConfig struct {
	Type       string           `yaml:"type"`
	SQLite     SQLiteConfig     `yaml:"sqlite"`
	PostgreSQL PostgreSQLConfig `yaml:"postgresql"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

type PostgreSQLConfig struct {
	DSN             string `yaml:"dsn"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime_seconds"`
}

type BackendsConfig struct {
	AllowDynamic bool            `yaml:"allow_dynamic"`
	List         []BackendConfig `yaml:"list"`
	Strategy     string          `yaml:"strategy"`
}

type BackendConfig struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Weight  int    `yaml:"weight"`
	Enabled bool   `yaml:"enabled"`
	Model   string `yaml:"model"`
	APIKey  string `yaml:"api_key"`
}

type MonitorConfig struct {
	RetentionDays      int   `yaml:"retention_days"`
	MaxRequestBytes    int   `yaml:"max_request_bytes"`
	MaxCaptureBytes    int   `yaml:"max_capture_bytes"`
	RequestTimeout     int   `yaml:"request_timeout_seconds"`
	PollBackendMetrics *bool `yaml:"poll_backend_metrics"`
	PollInterval       int   `yaml:"poll_interval_seconds"`
}

func loadYAMLConfig(path string) (*YAMLConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &YAMLConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	applyEnvOverrides(cfg)
	setConfigDefaults(cfg)

	return cfg, nil
}

func applyEnvOverrides(cfg *YAMLConfig) {
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.Server.ListenAddr = v
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.Server.DataDir = v
	}
	if v := os.Getenv("ALLOW_DYNAMIC_BACKEND"); v != "" {
		cfg.Backends.AllowDynamic = parseBoolEnv(v, cfg.Backends.AllowDynamic)
	}
	if v := os.Getenv("RETENTION_DAYS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Monitor.RetentionDays = i
		}
	}
	if v := os.Getenv("MAX_REQUEST_BYTES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Monitor.MaxRequestBytes = i
		}
	}
	if v := os.Getenv("MAX_CAPTURE_BYTES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Monitor.MaxCaptureBytes = i
		}
	}
	if v := os.Getenv("REQUEST_TIMEOUT_SECONDS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Monitor.RequestTimeout = i
		}
	}
	if v := os.Getenv("POLL_BACKEND_METRICS"); v != "" {
		b := parseBoolEnv(v, cfg.Monitor.PollBackendMetrics != nil && *cfg.Monitor.PollBackendMetrics)
		cfg.Monitor.PollBackendMetrics = &b
	}
	if v := os.Getenv("POLL_INTERVAL_SECONDS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Monitor.PollInterval = i
		}
	}
	if v := os.Getenv("DATABASE_TYPE"); v != "" {
		cfg.Database.Type = v
	}
	if v := os.Getenv("DATABASE_DSN"); v != "" {
		cfg.Database.PostgreSQL.DSN = v
	}
}

func setConfigDefaults(cfg *YAMLConfig) {
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = ":9091"
	}
	if cfg.Server.DataDir == "" {
		cfg.Server.DataDir = "./data"
	}
	if cfg.Database.Type == "" {
		cfg.Database.Type = "sqlite"
	}
	if cfg.Database.SQLite.Path == "" {
		cfg.Database.SQLite.Path = "monitor.db"
	}
	if cfg.Database.PostgreSQL.MaxOpenConns == 0 {
		cfg.Database.PostgreSQL.MaxOpenConns = 25
	}
	if cfg.Database.PostgreSQL.MaxIdleConns == 0 {
		cfg.Database.PostgreSQL.MaxIdleConns = 5
	}
	if cfg.Database.PostgreSQL.ConnMaxLifetime == 0 {
		cfg.Database.PostgreSQL.ConnMaxLifetime = 300
	}
	if cfg.Backends.Strategy == "" {
		cfg.Backends.Strategy = "wrr"
	}
	if cfg.Monitor.RetentionDays == 0 {
		cfg.Monitor.RetentionDays = 14
	}
	if cfg.Monitor.MaxRequestBytes == 0 {
		cfg.Monitor.MaxRequestBytes = 32 * 1024 * 1024
	}
	if cfg.Monitor.MaxCaptureBytes == 0 {
		cfg.Monitor.MaxCaptureBytes = 32 * 1024 * 1024
	}
	if cfg.Monitor.RequestTimeout == 0 {
		cfg.Monitor.RequestTimeout = 600
	}
	if cfg.Monitor.PollInterval == 0 {
		cfg.Monitor.PollInterval = 10
	}
	if cfg.Monitor.PollBackendMetrics == nil {
		v := true
		cfg.Monitor.PollBackendMetrics = &v
	}

	for i := range cfg.Backends.List {
		if cfg.Backends.List[i].Weight == 0 {
			cfg.Backends.List[i].Weight = 1
		}
		if !cfg.Backends.List[i].Enabled && cfg.Backends.List[i].Weight > 0 {
			cfg.Backends.List[i].Enabled = true
		}
	}
}

func parseBoolEnv(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func (c *YAMLConfig) toLegacyConfig() Config {
	return Config{
		ListenAddr:          c.Server.ListenAddr,
		AllowDynamicBackend: c.Backends.AllowDynamic,
		DataDir:             c.Server.DataDir,
		RetentionDays:       c.Monitor.RetentionDays,
		MaxRequestBytes:     int64(c.Monitor.MaxRequestBytes),
		MaxCaptureBytes:     int64(c.Monitor.MaxCaptureBytes),
		RequestTimeout:      time.Duration(c.Monitor.RequestTimeout) * time.Second,
		PollBackendMetrics:  c.Monitor.PollBackendMetrics != nil && *c.Monitor.PollBackendMetrics,
		PollInterval:        time.Duration(c.Monitor.PollInterval) * time.Second,
	}
}

func (c *YAMLConfig) getEnabledBackends() []BackendConfig {
	var enabled []BackendConfig
	for _, b := range c.Backends.List {
		if b.Enabled {
			enabled = append(enabled, b)
		}
	}
	return enabled
}

func (c *YAMLConfig) hasWeightedBackends() bool {
	return len(c.Backends.List) > 0
}
