package main

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
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
}

type Server struct {
	cfg      Config
	yamlCfg  *YAMLConfig
	db       Database
	balancer *BackendBalancer
	client   *http.Client
	hub      *EventHub
	active   atomic.Int64

	sseMu    sync.Mutex
	sseCancels map[context.Context]context.CancelFunc
}
type RequestRecord struct {
	ID                 string    `json:"id"`
	CreatedAt          time.Time `json:"created_at"`
	Method             string    `json:"method"`
	Path               string    `json:"path"`
	Query              string    `json:"query"`
	ClientIP           string    `json:"client_ip"`
	BackendURL         string    `json:"backend_url"`
	Model              string    `json:"model"`
	IsStreaming        bool      `json:"is_streaming"`
	StatusCode         int       `json:"status_code"`
	ErrorText          string    `json:"error_text"`
	RequestBytes       int64     `json:"request_bytes"`
	ResponseBytes      int64     `json:"response_bytes"`
	PromptTokens       int64     `json:"prompt_tokens"`
	CachedPromptTokens int64     `json:"cached_prompt_tokens"`
	CacheHitPct        float64   `json:"cache_hit_pct"`
	CompletionTokens   int64     `json:"completion_tokens"`
	TotalTokens        int64     `json:"total_tokens"`
	PromptMs           float64   `json:"prompt_ms"`
	CompletionMs       float64   `json:"completion_ms"`
	TotalMs            float64   `json:"total_ms"`
	FirstByteMs        float64   `json:"first_byte_ms"`
	ChunksCount        int64     `json:"chunks_count"`
	RequestRawPath     string    `json:"request_raw_path"`
	ResponseRawPath    string    `json:"response_raw_path"`
	UserAgent          string    `json:"user_agent"`
	PromptTokPerSec    float64   `json:"prompt_tok_per_sec"`
	DecodeTokPerSec    float64   `json:"decode_tok_per_sec"`
	TotalTokPerSec     float64   `json:"total_tok_per_sec"`
}

type RequestFilter struct {
	Path                string
	Model               string
	Method              string
	Backend             string
	Search              string
	StatusCode          int
	SinceHours          int
	Streaming           *bool
	ErrorsOnly          bool
	WithTokens          bool
	ChatCompletionsOnly bool
}
type responseMeta struct {
	Model              string
	PromptTokens       int64
	CachedPromptTokens int64
	CompletionTok      int64
	TotalTokens        int64
	PromptMs           float64
	CompletionMs       float64
}
type limitedBuffer struct {
	max int64
	buf bytes.Buffer
}

func newLimitedBuffer(max int64) *limitedBuffer {
	return &limitedBuffer{max: max}
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	if lb.max <= 0 {
		return lb.buf.Write(p)
	}
	remaining := lb.max - int64(lb.buf.Len())
	if remaining <= 0 {
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		_, _ = lb.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = lb.buf.Write(p)
	return len(p), nil
}

func (lb *limitedBuffer) Bytes() []byte {
	return lb.buf.Bytes()
}
