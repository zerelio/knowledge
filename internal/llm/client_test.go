package llm_test

import (
	"context"
	"testing"

	"github.com/zerelio/zerelio/internal/llm"
)

func TestNoopProvider(t *testing.T) {
	p := llm.NewFromEnv() // no env vars set in test → noop
	if !llm.IsNoop(p) {
		// If a provider is configured in the test environment, skip.
		t.Skip("LLM provider is configured; skipping noop test")
	}
	if p.Name() != "noop" {
		t.Errorf("Name = %q, want noop", p.Name())
	}
	_, err := p.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Error("noop Embed should return error")
	}
	_, err = p.Complete(context.Background(), "", []llm.Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Error("noop Complete should return error")
	}
}

func TestNewFromEnv_NoConfig(t *testing.T) {
	t.Setenv("ZERELIO_LLM_PROVIDER", "")
	t.Setenv("ZERELIO_LLM_API_KEY", "")
	p := llm.NewFromEnv()
	if !llm.IsNoop(p) {
		t.Error("expected noop when no provider configured")
	}
}

func TestNewFromEnv_MissingKey(t *testing.T) {
	t.Setenv("ZERELIO_LLM_PROVIDER", "openai")
	t.Setenv("ZERELIO_LLM_API_KEY", "")
	p := llm.NewFromEnv()
	if !llm.IsNoop(p) {
		t.Error("expected noop when openai key is missing")
	}
}

func TestNewFromEnv_OllamaNoKey(t *testing.T) {
	t.Setenv("ZERELIO_LLM_PROVIDER", "ollama")
	t.Setenv("ZERELIO_LLM_API_KEY", "")
	p := llm.NewFromEnv()
	if llm.IsNoop(p) {
		t.Error("ollama does not need an API key — should not be noop")
	}
	if p.Name() != "ollama" {
		t.Errorf("provider name = %q, want ollama", p.Name())
	}
}
