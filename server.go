package main

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	"llama_proxy/internal/balancer"
	"llama_proxy/internal/config"
	"llama_proxy/internal/events"
	"llama_proxy/internal/scheduler"
	"llama_proxy/internal/store"
)

// Server 是路由器核心对象，持有配置与各子系统（存储/负载均衡/调度/事件广播）。
// HTTP 入口见 router.go 的 gin 路由引擎；
// 后端池/调度的胶水逻辑（入池同步、metrics 轮询、事件广播）见 pool.go。
type Server struct {
	cfg            config.Config
	yamlCfg        *config.YAMLConfig
	store          *store.Store
	balancer       *balancer.Balancer
	scheduler      *scheduler.Scheduler
	staticBackends []config.BackendConfig
	// localNodeInPool 记录本地 background 节点是否已入池，避免无谓的池重建。
	localNodeInPool bool
	client          *http.Client
	hub             *events.EventHub
	active          atomic.Int64

	sseMu       sync.Mutex
	sseCancels  map[context.Context]context.CancelFunc
	schedMu     sync.RWMutex
	schedEvents []map[string]any

	// routerOnce/router 惰性构建并缓存 gin 路由引擎（见 router.go）。
	routerOnce sync.Once
	router     http.Handler
}

// newInflightCtx 返回一个带 request_id 的请求上下文，供 log.WithCtx 提取，
// 以便一个请求从接收到结束的所有日志能按 request_id 串联。
func (s *Server) newInflightCtx(parent context.Context, requestID string) context.Context {
	return context.WithValue(parent, "request_id", requestID)
}

// newSSECtx 为 SSE 长连接创建可统一取消的上下文，优雅起停时能立即断开监控连接。
func (s *Server) newSSECtx(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	s.sseMu.Lock()
	if s.sseCancels == nil {
		s.sseCancels = make(map[context.Context]context.CancelFunc)
	}
	s.sseCancels[ctx] = cancel
	s.sseMu.Unlock()

	return ctx, func() {
		cancel()
		s.sseMu.Lock()
		delete(s.sseCancels, ctx)
		s.sseMu.Unlock()
	}
}

// cancelAllSSE 取消所有 SSE 长连接，用于优雅起停时避免 Shutdown 傻等监控连接。
func (s *Server) cancelAllSSE() {
	s.sseMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.sseCancels))
	for _, c := range s.sseCancels {
		cancels = append(cancels, c)
	}
	s.sseMu.Unlock()

	for _, c := range cancels {
		c()
	}
}
