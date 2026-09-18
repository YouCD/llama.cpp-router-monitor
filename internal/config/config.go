// Package config 定义 YAML 配置结构与运行时（legacy）配置。
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr          string
	AllowDynamicBackend bool
	DataDir             string
	RetentionDays       int
	MaxRequestBytes     int64
	MaxCaptureBytes     int64
	RequestTimeout      time.Duration
	PollBackendMetrics  bool
	PollInterval        time.Duration
	RecordPaths         []string
	APIKey              string   // 客户端访问 proxy 的 API Key，空值则不鉴权
	UIAllowedHosts      []string // 允许访问 /_proxy/ui 的 Host 白名单，空则不限制
	LogLevel            string
}

type YAMLConfig struct {
	Server     ServerConfig      `yaml:"server"`
	Database   DatabaseConfig    `yaml:"database"`
	Backends   BackendsConfig    `yaml:"backends"`
	Proxy      ProxyConfig       `yaml:"proxy"`
	Routing    *RoutingConfig    `yaml:"routing"`
	Scheduling *SchedulingConfig `yaml:"scheduling"`
}

// RoutingConfig 描述按客户端特征路由到指定标签池的规则表。
// 规则按配置顺序评估，首条命中的规则生效；未配置时代理回退到内置的 GoClaw 规则
// （User-Agent 含 "GoClaw" → tool_call 池），保持既有行为。
type RoutingConfig struct {
	Rules []RoutingRule `yaml:"rules"`
}

// RoutingRule 描述一条路由规则：Match 条件命中后，请求只从 Pool 标签对应的后端子池
// 中选择，动态后端覆盖（X-Backend-URL / ?backend=）对该规则不生效；请求失败时自动
// 故障转移到子池内下一个未尝试的后端。
type RoutingRule struct {
	// Name 规则名，仅用于日志与报错；留空时按序号生成。
	Name string `yaml:"name"`
	// Match 规则命中条件，各条件同时配置时为 AND 关系。
	Match RuleMatch `yaml:"match"`
	// Pool 标签名：命中后只从携带该标签（tags 或 tool_call）的后端中选择。
	Pool string `yaml:"pool"`
}

// RuleMatch 描述规则命中条件。
type RuleMatch struct {
	// UserAgentContains 非空时要求 User-Agent 包含该子串（忽略大小写）。
	UserAgentContains string `yaml:"user_agent_contains"`
	// Headers 中任一请求头的值与配置值完全匹配即视为该条件命中（与 coding.header 同款语义）。
	Headers map[string]string `yaml:"header"`
}

// SchedulingConfig 描述进程调度子系统的配置。为 nil 时整个调度子系统禁用，
// 代理退化为纯转发模式（转发到 backends.list 外部后端）。
type SchedulingConfig struct {
	Coding     ProcessConfig `yaml:"coding"`
	Background ProcessConfig `yaml:"background"`
	// Lease 描述开发租约相关配置。
	Lease LeaseConfig `yaml:"lease"`
	// Switch 描述模型切换时的时序参数。
	Switch SwitchConfig `yaml:"switch"`
}

// ProcessConfig 描述模型进程的启动配置（coding/background 共用）。
// Header 仅对 coding 生效：允许配置多组，任一请求头的值与对应值完全匹配即判定为开发(coding)流量。
type ProcessConfig struct {
	Command      string            `yaml:"command"`
	Header       map[string]string `yaml:"header"`
	ReadinessURL string            `yaml:"readiness_url"`
	// APIKey 是模型进程自身的 API Key（对应 llama-server 的 --api-key）。
	// 非空时：就绪探测与代理转发均使用 Authorization: Bearer <key>，客户端 key 与后端 key 隔离。
	APIKey  string `yaml:"api_key"`
	LogFile string `yaml:"log_file"`
	// Weight 仅对 background 生效：本地 background 模型进程就绪后作为后端节点加入
	// 代理池（与 backends.list 一起负载均衡）时的权重，未配置按 1 处理。
	Weight int `yaml:"weight"`
	// ToolCall 仅对 background 生效：本地 background 模型支持工具调用（tool calls）。
	// 入池后 User-Agent 含 "GoClaw" 的请求（依赖工具调用）也会路由到它。
	ToolCall bool `yaml:"tool_call"`
	// Tags 仅对 background 生效：本地 background 节点入池后携带的路由标签，
	// routing 规则的 pool 可与之对应（tool_call: true 等价于含 "tool_call" 标签）。
	Tags []string `yaml:"tags"`
}

// EffectiveTags 返回本地 background 节点的有效标签集（语义同 BackendConfig.EffectiveTags）。
func (p *ProcessConfig) EffectiveTags() map[string]bool {
	tags := make(map[string]bool, len(p.Tags)+1)
	for _, t := range p.Tags {
		if t = strings.TrimSpace(t); t != "" {
			tags[t] = true
		}
	}
	if p.ToolCall {
		tags["tool_call"] = true
	}
	return tags
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
	LogLevel       string   `yaml:"log_level"`
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
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
	// Model 是该后端实际部署的模型 ID，转发时自动重写请求体的 model 字段。
	Model string `yaml:"model"`
	// APIKey 是该后端自身的 API Key，转发时自动注入 Authorization: Bearer <key>。
	APIKey string `yaml:"api_key"`
	// ToolCall 表示该后端模型支持工具调用（tool calls）。
	// User-Agent 含 "GoClaw" 的请求依赖工具调用，只路由到 tool_call: true 的后端。
	ToolCall bool `yaml:"tool_call"`
	// Tags 是该后端加入的路由标签池列表；routing 规则按 pool 标签选择后端。
	Tags []string `yaml:"tags"`
}

// EffectiveTags 返回后端的有效标签集：显式 tags 与 tool_call 布尔值的并集
// （tool_call: true 等价于含 "tool_call" 标签）。
func (b *BackendConfig) EffectiveTags() map[string]bool {
	tags := make(map[string]bool, len(b.Tags)+1)
	for _, t := range b.Tags {
		if t = strings.TrimSpace(t); t != "" {
			tags[t] = true
		}
	}
	if b.ToolCall {
		tags["tool_call"] = true
	}
	return tags
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

func Load(path string) (*YAMLConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &YAMLConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	setConfigDefaults(cfg)

	if err := validateRouting(cfg.Routing); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateRouting 校验 routing 规则：每条规则必须指定 pool，且 match 至少包含
// user_agent_contains 或 header 条件之一。
func validateRouting(rc *RoutingConfig) error {
	if rc == nil {
		return nil
	}
	for i, rr := range rc.Rules {
		if strings.TrimSpace(rr.Pool) == "" {
			return fmt.Errorf("routing.rules[%d]: pool 不能为空", i)
		}
		if rr.Match.UserAgentContains == "" && len(rr.Match.Headers) == 0 {
			return fmt.Errorf("routing.rules[%d]: match 至少需要 user_agent_contains 或 header", i)
		}
	}
	return nil
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

// HasScheduling 报告进程调度子系统是否启用。
func (c *YAMLConfig) HasScheduling() bool {
	return c.Scheduling != nil && (c.Scheduling.Coding.Command != "" || c.Scheduling.Background.Command != "")
}

func (c *YAMLConfig) ToLegacy() Config {
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
		LogLevel:            c.Server.LogLevel,
	}
}

func (c *YAMLConfig) HasWeightedBackends() bool {
	return len(c.Backends.List) > 0
}

// ValidateBackendURL 校验后端 URL 格式。
func ValidateBackendURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("backend URL is empty")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return fmt.Errorf("backend URL must start with http:// or https://")
	}
	return nil
}
