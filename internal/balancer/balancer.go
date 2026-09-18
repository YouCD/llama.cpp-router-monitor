// Package balancer 封装后端加权负载均衡（wrr/rr）。
package balancer

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"llama_proxy/internal/config"

	"github.com/fufuok/balancer"
)

type Balancer struct {
	mu       sync.RWMutex
	bal      balancer.Balancer
	backends map[string]*config.BackendConfig
	strategy string
	// logger 为 nil 时不打池变更日志（测试默认静默）。
	logger func(format string, args ...any)
	// pools 按标签划分的后端子池：routing 规则的 pool 字段引用标签名，命中规则的
	// 请求只从对应子池中选择。"tool_call" 子池即 tool_call: true（或 tags 含
	// "tool_call"）的后端，GoClaw 流量（User-Agent 含 "GoClaw"）只允许路由到该子池。
	pools map[string]balancer.Balancer
}

func New(backends []config.BackendConfig, strategy string) *Balancer {
	bb := &Balancer{
		backends: make(map[string]*config.BackendConfig),
		strategy: strategy,
	}
	bb.updateBalancer(backends)
	return bb
}

func (bb *Balancer) updateBalancer(backends []config.BackendConfig) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	bb.backends = make(map[string]*config.BackendConfig, len(backends))
	wNodes := map[string]int{}
	tagNodes := map[string]map[string]int{}

	for i := range backends {
		b := &backends[i]
		bb.backends[b.Name] = b
		wNodes[b.Name] = b.Weight
		for tag := range b.EffectiveTags() {
			if tagNodes[tag] == nil {
				tagNodes[tag] = map[string]int{}
			}
			tagNodes[tag][b.Name] = b.Weight
		}
	}

	bb.bal = newStrategyBalancer(bb.strategy, wNodes)
	bb.pools = make(map[string]balancer.Balancer, len(tagNodes))
	for tag, nodes := range tagNodes {
		bb.pools[tag] = newStrategyBalancer(bb.strategy, nodes)
	}

	bb.emitPoolLog()
}

// SetLogger 注入日志函数：池初始化/变更时输出 strategy 与每个标签池的成员及权重。
func (bb *Balancer) SetLogger(f func(format string, args ...any)) *Balancer {
	bb.logger = f
	return bb
}

// LogPools 主动输出一次当前池摘要（启动时使用，此时尚无 Update 事件）。
func (bb *Balancer) LogPools() {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	bb.emitPoolLogLocked()
}

// emitPoolLog 输出池摘要。调用方需持有写锁（updateBalancer）或读锁（LogPools）。
func (bb *Balancer) emitPoolLog() {
	bb.emitPoolLogLocked()
}

// emitPoolLogLocked 在已持锁状态下输出池摘要；未注入 logger 时静默。
func (bb *Balancer) emitPoolLogLocked() {
	if bb.logger == nil {
		return
	}
	bb.logger("backend pool: %s", bb.poolSummary())
}

// poolSummary 生成确定性的池摘要：strategy，随后是全量池（all）与每个标签池的
// 成员及权重（按名字典序排序）。
func (bb *Balancer) poolSummary() string {
	names := make([]string, 0, len(bb.backends))
	for name := range bb.backends {
		names = append(names, name)
	}
	sort.Strings(names)

	formatMembers := func(has func(*config.BackendConfig) bool) string {
		var m []string
		for _, name := range names {
			b := bb.backends[name]
			if has(b) {
				m = append(m, fmt.Sprintf("%s(w=%d)", name, b.Weight))
			}
		}
		if len(m) == 0 {
			return "(empty)"
		}
		return strings.Join(m, ",")
	}

	parts := make([]string, 0, len(bb.pools)+2)
	parts = append(parts, "strategy="+bb.strategy)
	parts = append(parts, "all="+formatMembers(func(*config.BackendConfig) bool { return true }))
	tags := make([]string, 0, len(bb.pools))
	for tag := range bb.pools {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		parts = append(parts, tag+"="+formatMembers(func(b *config.BackendConfig) bool {
			return b.EffectiveTags()[tag]
		}))
	}
	return strings.Join(parts, " ")
}

// newStrategyBalancer 按策略构造对应的 balancer。
func newStrategyBalancer(strategy string, wNodes map[string]int) balancer.Balancer {
	switch strategy {
	case "swrr":
		return balancer.NewSmoothWeightedRoundRobin(wNodes)
	case "wr":
		return balancer.NewWeightedRand(wNodes)
	case "rr":
		return balancer.NewRoundRobin(sortedNames(wNodes))
	case "random":
		return balancer.NewRandom(sortedNames(wNodes))
	default: // wrr or unknown
		return balancer.NewWeightedRoundRobin(wNodes)
	}
}

// sortedNames 返回按字典序排序的节点名，保证 rr/random 的节点顺序确定。
func sortedNames(m map[string]int) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// selectFrom 从给定子池按策略选一个后端；子池为空返回 nil。调用方需持有读锁。
func (bb *Balancer) selectFrom(bal balancer.Balancer) *config.BackendConfig {
	if bal == nil {
		return nil
	}
	name := bal.Select()
	if name == "" {
		return nil
	}
	if c, ok := bb.backends[name]; ok {
		return c
	}
	return nil
}

// Select 从全量池按策略选一个后端；无可用后端时返回 nil。
func (bb *Balancer) Select() *config.BackendConfig {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.selectFrom(bb.bal)
}

// SelectFromTag 从指定标签的后端子池按策略选一个后端；该标签无后端时返回 nil。
func (bb *Balancer) SelectFromTag(tag string) *config.BackendConfig {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.selectFrom(bb.pools[tag])
}

// SelectToolCall 从 tool_call 子池选一个后端（SelectFromTag("tool_call") 的兼容别名）。
func (bb *Balancer) SelectToolCall() *config.BackendConfig {
	return bb.SelectFromTag("tool_call")
}

func (bb *Balancer) Update(backends []config.BackendConfig) {
	bb.updateBalancer(backends)
}

func (bb *Balancer) GetBackendByName(name string) *config.BackendConfig {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.backends[name]
}

// Names 返回池内所有后端名（按字典序排序）。
func (bb *Balancer) Names() []string {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	names := make([]string, 0, len(bb.backends))
	for name := range bb.backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Len 返回池内后端数量。
func (bb *Balancer) Len() int {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return len(bb.backends)
}

func (bb *Balancer) GetStrategy() string {
	return strings.TrimSpace(bb.strategy)
}

// SelectExcluding 按策略选取一个不在 exclude 中的后端，用于某后端请求失败后的
// 自动故障转移。exclude 传入已尝试过的后端名；没有可用后端时返回 nil。
func (bb *Balancer) SelectExcluding(exclude map[string]bool) *config.BackendConfig {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.selectExcluding(bb.bal, exclude, nil)
}

// SelectFromTagExcluding 与 SelectExcluding 相同，但只在指定标签的子池中选取，
// 用于命中路由规则后的自动故障转移。
func (bb *Balancer) SelectFromTagExcluding(tag string, exclude map[string]bool) *config.BackendConfig {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.selectExcluding(bb.pools[tag], exclude, func(c *config.BackendConfig) bool {
		return c.EffectiveTags()[tag]
	})
}

// SelectToolCallExcluding 与 SelectFromTagExcluding 相同，限定 tool_call 子池（兼容别名）。
func (bb *Balancer) SelectToolCallExcluding(exclude map[string]bool) *config.BackendConfig {
	return bb.SelectFromTagExcluding("tool_call", exclude)
}

// selectExcluding 反复按策略选点，跳过 exclude 中的节点以及不满足 filter 的节点，
// 直到选到可用节点或达到尝试上限。上限取全部后端权重之和（一个完整 WRR 周期必然
// 覆盖所有节点，rr 与 random 同理），保证终止。调用方必须持有读锁。
func (bb *Balancer) selectExcluding(bal balancer.Balancer, exclude map[string]bool, filter func(*config.BackendConfig) bool) *config.BackendConfig {
	if bal == nil || len(bb.backends) == 0 {
		return nil
	}
	maxTries := 0
	for _, b := range bb.backends {
		if b.Weight > 0 {
			maxTries += b.Weight
		}
	}
	if maxTries <= 0 {
		maxTries = len(bb.backends)
	}
	for i := 0; i < maxTries; i++ {
		name := bal.Select()
		if name == "" || exclude[name] {
			continue
		}
		c, ok := bb.backends[name]
		if !ok || (filter != nil && !filter(c)) {
			continue
		}
		return c
	}
	return nil
}
