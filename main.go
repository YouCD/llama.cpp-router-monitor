package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/youcd/toolkit/log"
)

func main() {
	yamlPath := flag.String("f", "", "path to YAML config file")
	flag.Parse()

	configPath := *yamlPath
	if configPath == "" {
		configPath = "config.yaml"
	}

	log.Init(&log.Config{Stdout: true})

	yamlCfg, err := loadYAMLConfig(configPath)
	if err != nil {
		log.WithCtx(nil).Fatalf("load yaml config %s: %v", configPath, err)
	}

	cfg := yamlCfg.toLegacyConfig()

	if len(yamlCfg.getEnabledBackends()) == 0 && !yamlCfg.Backends.AllowDynamic {
		log.WithCtx(nil).Fatal("no enabled backend in backends.list and dynamic backend override is disabled")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.WithCtx(nil).Fatalf("mkdir data dir: %v", err)
	}

	db, err := NewDatabase(yamlCfg.Database, cfg.DataDir)
	if err != nil {
		log.WithCtx(nil).Fatalf("open db: %v", err)
	}
	defer db.Close()

	dbType := yamlCfg.Database.Type
	if err := InitDB(db, dbType); err != nil {
		log.WithCtx(nil).Fatalf("init db: %v", err)
	}
	if err := normalizeDB(db); err != nil {
		log.WithCtx(nil).Fatalf("normalize db: %v", err)
	}
	if err := repairStuckRequests(db, cfg.DataDir); err != nil {
		log.WithCtx(nil).Infof("repair stuck requests failed: %v", err)
	}

	var backendBalancer *BackendBalancer
	if yamlCfg.hasWeightedBackends() {
		backendBalancer = NewBackendBalancer(yamlCfg.getEnabledBackends(), yamlCfg.Backends.Strategy)
		log.WithCtx(nil).Infof("load balancer initialized with strategy: %s, backends: %d", yamlCfg.Backends.Strategy, backendBalancer.GetEnabledCount())
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}

	s := &Server{
		cfg:      cfg,
		yamlCfg:  yamlCfg,
		db:       db,
		balancer: backendBalancer,
		client: &http.Client{
			Transport: transport,
			Timeout:   cfg.RequestTimeout,
		},
		hub: NewEventHub(),
	}

	// 捕获 SIGINT / SIGTERM，用于优雅起停。
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go s.cleanupLoop(ctx)
	if cfg.PollBackendMetrics {
		go s.backendMetricsLoop(ctx)
	}

	backendCount := 0
	if s.balancer != nil {
		backendCount = s.balancer.GetEnabledCount()
	}
	log.WithCtx(ctx).Infof("llama.cpp Router Monitor listening on %s, backends=%d", cfg.ListenAddr, backendCount)

	server := &http.Server{Addr: cfg.ListenAddr, Handler: s}
	// 优雅起停时先断开 SSE 长连接，避免 Shutdown 傻等监控页面连接直到超时。
	server.RegisterOnShutdown(s.cancelAllSSE)
	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			log.WithCtx(ctx).Fatalf("server failed: %v", err)
		}
	case <-ctx.Done():
		// 收到退出信号：停止接收新连接并排空在途请求。
		log.WithCtx(ctx).Infof("shutdown signal received, draining in-flight requests")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.WithCtx(ctx).Infof("graceful shutdown error: %v", err)
		}
		cancel()
		log.WithCtx(ctx).Infof("shutdown complete")
	}
}
