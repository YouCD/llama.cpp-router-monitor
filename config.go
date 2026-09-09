package main

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type YAMLConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Backends BackendsConfig `yaml:"backends"`
	Proxy    ProxyConfig    `yaml:"proxy"`
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

type ProxyConfig struct {
	RetentionDays      int      `yaml:"retention_days"`
	MaxRequestBytes    int      `yaml:"max_request_bytes"`
	MaxCaptureBytes    int      `yaml:"max_capture_bytes"`
	RequestTimeout     int      `yaml:"request_timeout_seconds"`
	PollBackendMetrics *bool    `yaml:"poll_backend_metrics"`
	PollInterval       int      `yaml:"poll_interval_seconds"`
	RecordPaths        []string `yaml:"record_paths"`
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

	setConfigDefaults(cfg)

	return cfg, nil
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
		cfg.Database.SQLite.Path = "proxy.db"
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
	if cfg.Proxy.RetentionDays == 0 {
		cfg.Proxy.RetentionDays = 14
	}
	if cfg.Proxy.MaxRequestBytes == 0 {
		cfg.Proxy.MaxRequestBytes = 32 * 1024 * 1024
	}
	if cfg.Proxy.MaxCaptureBytes == 0 {
		cfg.Proxy.MaxCaptureBytes = 32 * 1024 * 1024
	}
	if cfg.Proxy.RequestTimeout == 0 {
		cfg.Proxy.RequestTimeout = 600
	}
	if cfg.Proxy.PollInterval == 0 {
		cfg.Proxy.PollInterval = 10
	}
	if cfg.Proxy.PollBackendMetrics == nil {
		v := true
		cfg.Proxy.PollBackendMetrics = &v
	}
	if len(cfg.Proxy.RecordPaths) == 0 {
		cfg.Proxy.RecordPaths = []string{"/v1/chat/completions", "/v1/completions", "/v1/embeddings"}
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

func (c *YAMLConfig) toLegacyConfig() Config {
	return Config{
		ListenAddr:          c.Server.ListenAddr,
		AllowDynamicBackend: c.Backends.AllowDynamic,
		DataDir:             c.Server.DataDir,
		RetentionDays:       c.Proxy.RetentionDays,
		MaxRequestBytes:     int64(c.Proxy.MaxRequestBytes),
		MaxCaptureBytes:     int64(c.Proxy.MaxCaptureBytes),
		RequestTimeout:      time.Duration(c.Proxy.RequestTimeout) * time.Second,
		PollBackendMetrics:  c.Proxy.PollBackendMetrics != nil && *c.Proxy.PollBackendMetrics,
		PollInterval:        time.Duration(c.Proxy.PollInterval) * time.Second,
		RecordPaths:         c.Proxy.RecordPaths,
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
