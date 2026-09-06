package main

import (
	"sync"

	"github.com/fufuok/balancer"
)

type BackendBalancer struct {
	mu       sync.RWMutex
	bal      balancer.Balancer
	backends map[string]*BackendConfig
	strategy string
}

func NewBackendBalancer(backends []BackendConfig, strategy string) *BackendBalancer {
	bb := &BackendBalancer{
		backends: make(map[string]*BackendConfig),
		strategy: strategy,
	}
	bb.updateBalancer(backends)
	return bb
}

func (bb *BackendBalancer) updateBalancer(backends []BackendConfig) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	bb.backends = make(map[string]*BackendConfig)
	wNodes := map[string]int{}

	for i := range backends {
		b := &backends[i]
		if b.Enabled {
			bb.backends[b.Name] = b
			wNodes[b.Name] = b.Weight
		}
	}

	switch bb.strategy {
	case "swrr":
		bb.bal = balancer.NewSmoothWeightedRoundRobin(wNodes)
	case "wr":
		bb.bal = balancer.NewWeightedRand(wNodes)
	case "rr":
		nodes := make([]string, 0, len(wNodes))
		for k := range wNodes {
			nodes = append(nodes, k)
		}
		bb.bal = balancer.NewRoundRobin(nodes)
	case "random":
		nodes := make([]string, 0, len(wNodes))
		for k := range wNodes {
			nodes = append(nodes, k)
		}
		bb.bal = balancer.NewRandom(nodes)
	case "wrr", "":
		bb.bal = balancer.NewWeightedRoundRobin(wNodes)
	default:
		bb.bal = balancer.NewWeightedRoundRobin(wNodes)
	}
}

func (bb *BackendBalancer) Select() *BackendConfig {
	bb.mu.RLock()
	defer bb.mu.RUnlock()

	if bb.bal == nil {
		return nil
	}

	name := bb.bal.Select()
	if name == "" {
		return nil
	}

	if b, ok := bb.backends[name]; ok {
		return b
	}
	return nil
}

func (bb *BackendBalancer) Update(backends []BackendConfig) {
	bb.updateBalancer(backends)
}

func (bb *BackendBalancer) GetBackendByName(name string) *BackendConfig {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.backends[name]
}

func (bb *BackendBalancer) Names() []string {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	names := make([]string, 0, len(bb.backends))
	for name := range bb.backends {
		names = append(names, name)
	}
	return names
}

func (bb *BackendBalancer) GetEnabledCount() int {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return len(bb.backends)
}

func (bb *BackendBalancer) GetStrategy() string {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return bb.strategy
}
