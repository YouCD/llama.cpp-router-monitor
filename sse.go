package main

import (
	"encoding/json"
	"sync"
)

type EventHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{clients: map[chan string]struct{}{}}
}

func (h *EventHub) Subscribe() chan string {
	ch := make(chan string, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *EventHub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *EventHub) Broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	msg := string(b)

	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}
