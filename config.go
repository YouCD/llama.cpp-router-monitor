package main

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type YAMLConfig struct {
	Server     ServerConfig      `yaml:"server"`
	Database   DatabaseConfig    `yaml:"database"`
	Backends   BackendsConfig    `yaml:"backends"`
	Proxy      ProxyConfig       `yaml:"proxy"`
	Scheduling *SchedulingConfig `yaml:"scheduling"`
}

// SchedulingConfig 描述进程调度子系统的配置。为 nil 时整个调度子系统禁用，
// 代理退化为纯转发模式（转发到 backends.list 外部后端）。
type SchedulingConfig struct {
	Coding CodingConfig `yaml:"coding"`
	// Background 描述后台模型的启动配置。
	Background BackgroundConfig `yaml:"background"`
	// Lease 描述开发租约相关配置。
	Lease LeaseConfig `yaml:"lease"`
	// Switch 描述模型切换时的时序参数。
	Switch SwitchConfig `yaml:"switch"`
}

// CodingConfig 描述开发模型的启动配置与识别头。
// header 允许配置多组：任一请求头的值与对应值完全匹配即判定为开发(coding)流量。
type CodingConfig struct {
	Command                string            `yaml:"command"`
	Header                 map[string]string `yaml:"header"`
	ReadinessURL           string            `yaml:"readiness_url"`
	ReadinessAuthorization string            `yaml:"readiness_authorization"`
	LogFile                string            `yaml:"log_file"`
}

// BackgroundConfig 描述后台模型的启动配置。
type BackgroundConfig struct {
	Command                string `yaml:"command"`
	ReadinessURL           string `yaml:"readiness_url"`
	ReadinessAuthorization string `yaml:"readiness_authorization"`
	LogFile                string `yaml:"log_file"`
}

// LeaseConfig 描述开发租约相关配置。
type LeaseConfig struct {
	// CodingIdleTimeout 距最后一次 coding 流量超过该时长即切回 background。
	CodingIdleTimeout time.Duration `yaml:"coding_idle_timeout"`
}

// SwitchConfig 描述模型切换时的时序参数。
type SwitchConfig struct {
	DrainTimeout   time.Duration `yaml:"drain_timeout"`
	KillTimeout    time.Duration `yaml:"kill_timeout"`
	StartupTimeout time.Duration `yaml:"startup_timeout"`
}

type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	DataDir    string `yaml:"data_dir"`
	// UIAllowedHosts 允许访问 /_proxy/ui 面板的 Host 列表（忽略端口与大小写）。
	// 为空则不限制；配置后，Host 不在列表内的请求访问面板将返回 403。
	UIAllowedHosts []string `yaml:"ui_allowed_hosts"`
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
	APIKey             string   `yaml:"api_key"` // 客户端访问 proxy 的 API Key，与后端 api_key 隔离
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

	if cfg.Scheduling != nil {
		if cfg.Scheduling.Coding.Header == nil {
			cfg.Scheduling.Coding.Header = map[string]string{"X-LLM-Purpose": "coding"}
		}
		if cfg.Scheduling.Coding.ReadinessURL == "" {
			cfg.Scheduling.Coding.ReadinessURL = "http://127.0.0.1:8080"
		}
		if cfg.Scheduling.Background.ReadinessURL == "" {
			cfg.Scheduling.Background.ReadinessURL = "http://127.0.0.1:8080"
		}
		if cfg.Scheduling.Lease.CodingIdleTimeout == 0 {
			cfg.Scheduling.Lease.CodingIdleTimeout = 30 * time.Minute
		}
		if cfg.Scheduling.Switch.DrainTimeout == 0 {
			cfg.Scheduling.Switch.DrainTimeout = 10 * time.Second
		}
		if cfg.Scheduling.Switch.KillTimeout == 0 {
			cfg.Scheduling.Switch.KillTimeout = 10 * time.Second
		}
		if cfg.Scheduling.Switch.StartupTimeout == 0 {
			cfg.Scheduling.Switch.StartupTimeout = 120 * time.Second
		}
	}
}

// hasScheduling 报告进程调度子系统是否启用。
func (c *YAMLConfig) hasScheduling() bool {
	return c.Scheduling != nil && (c.Scheduling.Coding.Command != "" || c.Scheduling.Background.Command != "")
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
		APIKey:              c.Proxy.APIKey,
		UIAllowedHosts:      c.Server.UIAllowedHosts,
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
