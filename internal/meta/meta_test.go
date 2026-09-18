package meta

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseResponseMetaJSON(t *testing.T) {
	body := []byte(`{
		"model":"llama-actual",
		"usage":{"prompt_tokens":481,"completion_tokens":712,"total_tokens":1193,"prompt_tokens_details":{"cached_tokens":88}},
		"timings":{"prompt_ms":120.5,"predicted_ms":356.25}
	}`)
	meta := ParseResponseMeta(http.Header{"Content-Type": []string{"application/json"}}, body)
	if meta.Model != "llama-actual" {
		t.Fatalf("model=%q", meta.Model)
	}
	if meta.PromptTokens != 481 || meta.CompletionTok != 712 || meta.TotalTokens != 1193 {
		t.Fatalf("unexpected tokens: %+v", meta)
	}
	if meta.CachedPromptTokens != 88 {
		t.Fatalf("unexpected cached tokens: %+v", meta)
	}
	if meta.PromptMs != 120.5 || meta.CompletionMs != 356.25 {
		t.Fatalf("unexpected timings: %+v", meta)
	}
}

func TestParseResponseMetaSSE(t *testing.T) {
	body := []byte(strings.Join([]string{
		`data: {"model":"stream-model","choices":[{"delta":{"content":"hi"}}]}`,
		`data: {"usage":{"prompt_tokens":12,"completion_tokens":34,"total_tokens":46},"timings":{"prompt_ms":20,"predicted_ms":80}}`,
		`data: [DONE]`,
		"",
	}, "\n"))
	meta := ParseResponseMeta(http.Header{"Content-Type": []string{"text/event-stream"}}, body)
	if meta.Model != "stream-model" {
		t.Fatalf("model=%q", meta.Model)
	}
	if meta.PromptTokens != 12 || meta.CompletionTok != 34 || meta.TotalTokens != 46 {
		t.Fatalf("unexpected tokens: %+v", meta)
	}
}

func TestParseResponseMetaSSEKeepsLaterPromptTokens(t *testing.T) {
	body := []byte(strings.Join([]string{
		`data: {"model":"stream-model","timings":{"prompt_n":0,"predicted_n":0}}`,
		`data: {"usage":{"prompt_tokens":19,"completion_tokens":712,"total_tokens":731,"prompt_tokens_details":{"cached_tokens":3}},"timings":{"prompt_n":19,"predicted_n":712,"prompt_ms":42,"predicted_ms":1337}}`,
		`data: [DONE]`,
		"",
	}, "\n"))
	meta := ParseResponseMeta(http.Header{"Content-Type": []string{"text/event-stream"}}, body)
	if meta.Model != "stream-model" {
		t.Fatalf("model=%q", meta.Model)
	}
	if meta.PromptTokens != 19 || meta.CompletionTok != 712 || meta.TotalTokens != 731 {
		t.Fatalf("unexpected tokens: %+v", meta)
	}
	if meta.CachedPromptTokens != 3 {
		t.Fatalf("unexpected cached tokens: %+v", meta)
	}
	if meta.PromptMs != 42 || meta.CompletionMs != 1337 {
		t.Fatalf("unexpected timings: %+v", meta)
	}
}

func TestParseResponseMetaLlamaCppFallbackFields(t *testing.T) {
	body := []byte(`{
		"model":"llama-fallback",
		"tokens_evaluated": 77,
		"tokens_predicted": 123,
		"tokens_evaluated_ms": 910.5,
		"tokens_predicted_ms": 2222.25
	}`)
	meta := ParseResponseMeta(http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}, body)
	if meta.Model != "llama-fallback" {
		t.Fatalf("model=%q", meta.Model)
	}
	if meta.PromptTokens != 77 || meta.CompletionTok != 123 || meta.TotalTokens != 200 {
		t.Fatalf("unexpected tokens: %+v", meta)
	}
	if meta.PromptMs != 910.5 || meta.CompletionMs != 2222.25 {
		t.Fatalf("unexpected timings: %+v", meta)
	}
}

func TestRewriteModel(t *testing.T) {
	body := []byte(`{"model":"client-model","messages":[]}`)
	newBody, model, err := RewriteModel(body, "qwen")
	if err != nil {
		t.Fatalf("rewriteModel: %v", err)
	}
	if model != "qwen" {
		t.Fatalf("model=%q", model)
	}
	if !strings.Contains(string(newBody), `"model":"qwen"`) {
		t.Fatalf("body not rewritten: %s", newBody)
	}
	if strings.Contains(string(newBody), "client-model") {
		t.Fatalf("old model still present: %s", newBody)
	}
}

func TestRewriteModelNonJSON(t *testing.T) {
	body := []byte("not-json")
	newBody, model, err := RewriteModel(body, "qwen")
	if err == nil {
		t.Fatalf("expected error for non-JSON, got model=%q body=%s", model, newBody)
	}
}
