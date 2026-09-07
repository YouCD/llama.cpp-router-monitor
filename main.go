package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	yamlPath := flag.String("f", "", "path to YAML config file")
	flag.Parse()

	configPath := *yamlPath
	if configPath == "" {
		configPath = "config.yaml"
	}

	yamlCfg, err := loadYAMLConfig(configPath)
	if err != nil {
		log.Fatalf("load yaml config %s: %v", configPath, err)
	}

	cfg := yamlCfg.toLegacyConfig()

	if len(yamlCfg.getEnabledBackends()) == 0 && !yamlCfg.Backends.AllowDynamic {
		log.Fatal("no enabled backend in backends.list and dynamic backend override is disabled")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("mkdir data dir: %v", err)
	}

	db, err := NewDatabase(yamlCfg.Database, cfg.DataDir)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	dbType := yamlCfg.Database.Type
	if err := InitDB(db, dbType); err != nil {
		log.Fatalf("init db: %v", err)
	}
	if err := normalizeDB(db); err != nil {
		log.Fatalf("normalize db: %v", err)
	}
	if err := repairStuckRequests(db, cfg.DataDir); err != nil {
		log.Printf("repair stuck requests failed: %v", err)
	}

	var backendBalancer *BackendBalancer
	if yamlCfg.hasWeightedBackends() {
		backendBalancer = NewBackendBalancer(yamlCfg.getEnabledBackends(), yamlCfg.Backends.Strategy)
		log.Printf("load balancer initialized with strategy: %s, backends: %d", yamlCfg.Backends.Strategy, backendBalancer.GetEnabledCount())
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.cleanupLoop(ctx)
	if cfg.PollBackendMetrics {
		go s.backendMetricsLoop(ctx)
	}

	backendCount := 0
	if s.balancer != nil {
		backendCount = s.balancer.GetEnabledCount()
	}
	log.Printf("llama.cpp Router Monitor listening on %s, backends=%d", cfg.ListenAddr, backendCount)
	if err := http.ListenAndServe(cfg.ListenAddr, s); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
