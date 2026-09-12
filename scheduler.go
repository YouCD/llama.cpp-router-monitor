package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/youcd/toolkit/log"
)

const (
	modeCoding     = "coding"
	modeBackground = "background"
)

// Scheduler 负责根据 opencode 的开发流量启动/切换 llama.cpp 模型进程。
// 仅在配置了 scheduling 时由 main 装配启用；未启用时为 nil，代理退化为纯转发。
type Scheduler struct {
	cfg    *SchedulingConfig
	lease  time.Duration
	sw     SwitchConfig
	client *http.Client

	// activeCount 返回当前在途请求数，用于切换时的优雅排空。
	activeCount func() int64
	infof       func(format string, args ...any)
	probe       func(ctx context.Context, url string) bool

	mu            sync.Mutex
	mode          string
	readyOK       bool
	readyCh       chan struct{}
	readyChClosed bool
	cmd           *exec.Cmd
	logFile       *os.File
	lastCodingAt  time.Time
	started       bool
}

// NewScheduler 基于调度配置创建一个调度器。默认日志走全局 log。
func NewScheduler(cfg *SchedulingConfig, lease time.Duration, sw SwitchConfig, client *http.Client) *Scheduler {
	s := &Scheduler{
		cfg:    cfg,
		lease:  lease,
		sw:     sw,
		client: client,
		infof:  func(format string, args ...any) { log.WithCtx(nil).Infof("[scheduler] "+format, args...) },
	}
	return s
}

// SetProbe 覆盖就绪探测函数（主要用于测试）。
func (s *Scheduler) SetProbe(f func(ctx context.Context, url string) bool) *Scheduler {
	s.probe = f
	return s
}

// SetActiveCount 注入在途请求计数函数，用于切换时优雅排空。
func (s *Scheduler) SetActiveCount(f func() int64) *Scheduler {
	s.activeCount = f
	return s
}

// SetLogger 覆盖内部日志函数（主要用于测试）。
func (s *Scheduler) SetLogger(f func(format string, args ...any)) *Scheduler {
	s.infof = f
	return s
}

// Start 启动调度器：首次拉起 background 进程并等待就绪。
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()
	if _, err := s.ensureTarget(ctx, modeBackground, s.cfg.Background.Command, s.cfg.Background.ReadinessURL, s.cfg.Background.ReadinessAuthorization, s.cfg.Background.LogFile); err != nil {
		return fmt.Errorf("initial background start: %w", err)
	}
	return nil
}

// Shutdown 停止当前运行的子进程（优雅退出时调用）。
func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	cmd := s.cmd
	logFile := s.logFile
	s.logFile = nil
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if logFile != nil {
		_ = logFile.Close()
	}
}

// codingHeaderMatch 判定一个请求是否为开发(coding)流量。
// 任一配置的 header 名称存在且其值与配置值完全匹配即为开发流量。
func (s *Scheduler) codingHeaderMatch(h http.Header) bool {
	for name, want := range s.cfg.Coding.Header {
		if v := strings.TrimSpace(h.Get(name)); v != "" && v == want {
			return true
		}
	}
	return false
}

// EnsureCoding 确保 coding 进程已就绪。若当前不在 coding 模式或未就绪则触发切换，
// 并返回一个就绪信号 channel：channel 关闭即代表 coding 进程可用。调用方等待该 channel。
// 并发调用会复用同一次切换，避免重复启动进程。
func (s *Scheduler) EnsureCoding(ctx context.Context) (chan struct{}, error) {
	s.mu.Lock()
	if s.mode == modeCoding && s.readyOK {
		ch := s.readyCh
		s.mu.Unlock()
		return ch, nil
	}
	// 已处于切换过程中的请求直接等待同一个 channel。
	if s.mode == modeCoding && !s.readyOK && s.readyCh != nil {
		ch := s.readyCh
		s.mu.Unlock()
		return ch, nil
	}
	s.mu.Unlock()

	ready, err := s.ensureTarget(ctx, modeCoding, s.cfg.Coding.Command, s.cfg.Coding.ReadinessURL, s.cfg.Coding.ReadinessAuthorization, s.cfg.Coding.LogFile)
	return ready, err
}

// TouchCoding 刷新开发租约时间。每次收到开发流量都应调用。
func (s *Scheduler) TouchCoding() {
	s.mu.Lock()
	s.lastCodingAt = time.Now()
	s.mu.Unlock()
}

// IsCodingActive 报告当前是否处于开发状态：即正在 coding 模式，或距离最后一次开发流量仍在租约内。
func (s *Scheduler) IsCodingActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode == modeCoding {
		return true
	}
	if s.lease > 0 && !s.lastCodingAt.IsZero() && time.Since(s.lastCodingAt) < s.lease {
		return true
	}
	return false
}

// ActiveBaseURL 返回当前已就绪模式的转发基址（含 /v1 前缀），用于代理转发。
// 若当前无就绪进程则返回空串。
func (s *Scheduler) ActiveBaseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.readyOK {
		return ""
	}
	if s.mode == modeCoding {
		return apiBaseFromReadiness(s.cfg.Coding.ReadinessURL)
	}
	return apiBaseFromReadiness(s.cfg.Background.ReadinessURL)
}

// apiBaseFromReadiness 从就绪探测地址推导 llama-server API 基址（scheme://host:port/v1）。
// readiness_url 可能带任意路径（如 /health、/v1/models），仅取其 origin 部分并拼上 /v1，
// 避免路径后缀污染转发地址。解析失败时返回空串。
func apiBaseFromReadiness(readinessURL string) string {
	u, err := url.Parse(strings.TrimRight(readinessURL, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/v1"
}

// Loop 运行租约超时检查与周期性就绪探活：距最后一次开发流量超过 lease 则切回
// background；同时每 5s 对当前模式进程做就绪探测，恢复因启动超时误标为未就绪的进程。
func (s *Scheduler) Loop(ctx context.Context) {
	if s.lease <= 0 {
		return
	}
	leaseTicker := time.NewTicker(s.lease)
	defer leaseTicker.Stop()
	probeTicker := time.NewTicker(5 * time.Second)
	defer probeTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-probeTicker.C:
			s.reconcileReady()
		case <-leaseTicker.C:
			s.mu.Lock()
			idle := s.mode == modeCoding && !s.lastCodingAt.IsZero() && time.Since(s.lastCodingAt) >= s.lease
			s.mu.Unlock()
			if idle {
				s.infof("coding idle timeout reached, switching to background")
				_, _ = s.ensureTarget(context.Background(), modeBackground, s.cfg.Background.Command, s.cfg.Background.ReadinessURL, s.cfg.Background.ReadinessAuthorization, s.cfg.Background.LogFile)
			}
		}
	}
}

// ensureTarget 将当前进程切换到 target 模式并等待就绪。
// 通过内部锁串行化切换，防止并发重复切换。
func (s *Scheduler) ensureTarget(ctx context.Context, target, command, readinessURL, readinessAuth, logFile string) (chan struct{}, error) {
	// 二次检查，避免竞态下重复切换。
	s.mu.Lock()
	if s.mode == target && s.readyOK {
		ch := s.readyCh
		s.mu.Unlock()
		return ch, nil
	}
	s.mu.Unlock()

	s.infof("switching to %s", target)

	// 优雅排空在途请求。
	s.drain()

	// 停止旧进程。
	s.stopProcess()

	// 打开新进程的日志文件（若配置了）。
	f, err := openProcessLog(logFile)
	if err != nil {
		s.infof("open log file %s failed: %v", logFile, err)
	}
	if f != nil {
		s.mu.Lock()
		s.logFile = f
		s.mu.Unlock()
	}

	newReady := make(chan struct{})
	s.mu.Lock()
	s.mode = target
	s.readyOK = false
	s.readyCh = newReady
	s.readyChClosed = false
	s.mu.Unlock()

	if command != "" {
		cmd := exec.Command("/bin/sh", "-c", command)
		if f != nil {
			cmd.Stdout = f
			cmd.Stderr = f
		}
		if err := cmd.Start(); err != nil {
			s.infof("start %s command failed: %v", target, err)
			s.markReadyClosed(newReady)
			return newReady, err
		}
		s.infof("%s process started pid=%d log=%v", target, cmd.Process.Pid, f != nil)
		s.mu.Lock()
		s.cmd = cmd
		s.mu.Unlock()
		go s.reap(cmd)
	}

	// 等待就绪。
	if err := s.waitReady(ctx, readinessURL, readinessAuth, s.sw.StartupTimeout); err != nil {
		s.infof("%s ready probe failed (will reconcile later): %v", target, err)
		s.mu.Lock()
		s.readyOK = false
		s.mu.Unlock()
		// 失败时不再关闭 newReady，交由 reconcileReady 在进程真正就绪后放行等待者。
		return newReady, err
	}

	s.mu.Lock()
	s.readyOK = true
	now := time.Now()
	if target == modeCoding {
		s.lastCodingAt = now
	}
	s.mu.Unlock()
	s.infof("%s ready", target)
	s.markReadyClosed(newReady)
	return newReady, nil
}

// markReadyClosed 安全地关闭 ready channel 并记录其已关闭状态，避免重复 close。
// 必须在持有 s.mu 之外调用。
func (s *Scheduler) markReadyClosed(ch chan struct{}) {
	s.mu.Lock()
	s.readyChClosed = true
	s.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// reconcileReady 周期性检查当前模式进程的就绪状态，用于修复"大模型启动慢导致
// 首次 waitReady 超时后进程实际就绪却一直标记为未就绪"的问题。就绪时恢复
// readyOK 并放行等待就绪信号的调用方。
func (s *Scheduler) reconcileReady() {
	s.mu.Lock()
	if s.readyOK || s.cmd == nil {
		s.mu.Unlock()
		return
	}
	mode := s.mode
	ch := s.readyCh
	closed := s.readyChClosed
	var baseURL, auth string
	if mode == modeCoding {
		baseURL = s.cfg.Coding.ReadinessURL
		auth = s.cfg.Coding.ReadinessAuthorization
	} else {
		baseURL = s.cfg.Background.ReadinessURL
		auth = s.cfg.Background.ReadinessAuthorization
	}
	s.mu.Unlock()

	if baseURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if s.probeReady(ctx, baseURL, auth) {
		s.mu.Lock()
		if !s.readyOK {
			s.readyOK = true
			s.infof("%s became ready (reconciled)", mode)
			if ch != nil && !closed {
				s.readyChClosed = true
				close(ch)
			}
		}
		s.mu.Unlock()
	}
}

// openProcessLog 以追加模式打开进程日志文件；logFile 为空时返回 (nil, nil)。
func openProcessLog(logFile string) (*os.File, error) {
	if logFile == "" {
		return nil, nil
	}
	if dir := filepath.Dir(logFile); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// drain 等待在途请求排空，受 drain_timeout 限制。
func (s *Scheduler) drain() {
	if s.activeCount == nil || s.sw.DrainTimeout <= 0 {
		return
	}
	deadline := time.Now().Add(s.sw.DrainTimeout)
	for {
		if s.activeCount() <= 0 {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// stopProcess 停止当前子进程，受 kill_timeout 限制。
func (s *Scheduler) stopProcess() {
	s.mu.Lock()
	cmd := s.cmd
	s.cmd = nil
	logFile := s.logFile
	s.logFile = nil
	s.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(s.sw.KillTimeout):
		}
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	s.infof("stopped previous process")
}

// reap 回收已退出的子进程，防止僵尸进程。
func (s *Scheduler) reap(cmd *exec.Cmd) {
	_ = cmd.Wait()
}

// waitReady 轮询配置的 readinessURL 本身，直到成功或超时。
func (s *Scheduler) waitReady(ctx context.Context, baseURL, authorization string, timeout time.Duration) error {
	if baseURL == "" {
		return fmt.Errorf("readiness_url is empty")
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if s.probeReady(probeCtx, baseURL, authorization) {
			return nil
		}
		select {
		case <-probeCtx.Done():
			return probeCtx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) probeReady(ctx context.Context, url, authorization string) bool {
	if s.probe != nil {
		return s.probe(ctx, url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

// getReadyCh 返回当前就绪信号（测试/状态查询用）。
func (s *Scheduler) getReadyCh() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readyCh
}
