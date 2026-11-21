// Package llm provides a unified interface to LLM and embedding providers.
// Supported: OpenAI, Anthropic, Ollama (local).
//
// Configuration via environment variables:
//
//	ZERELIO_LLM_PROVIDER     = openai | anthropic | ollama  (default: openai)
//	ZERELIO_LLM_API_KEY      = sk-...                       (required for openai/anthropic)
//	ZERELIO_LLM_MODEL        = gpt-4o-mini                  (completion model)
//	ZERELIO_EMBEDDING_MODEL  = text-embedding-3-small       (embedding model)
//	ZERELIO_LLM_BASE_URL     = http://localhost:11434       (ollama base URL)
//
// If ZERELIO_LLM_PROVIDER is unset or the API key is missing, NewFromEnv
// returns a NoopProvider that returns errors — the platform starts fine but
// AI features degrade gracefully.
package llm

import (
	"context"
	"os"
)

// Message is a single turn in a chat conversation.
type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// Provider is the unified LLM + embedding API abstraction.
type Provider interface {
	// Embed returns one embedding vector per input text.
	// All vectors have the same dimensionality (depends on the model).
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Complete calls a chat completion with an optional system prompt.
	Complete(ctx context.Context, system string, messages []Message) (string, error)

	// Name identifies the provider (e.g. "openai", "anthropic", "ollama").
	Name() string

	// EmbeddingDim returns the vector dimensionality for the configured model.
	EmbeddingDim() int
}

// Config holds provider configuration, normally loaded from environment.
type Config struct {
	Provider       string // openai | anthropic | ollama
	APIKey         string
	Model          string // chat/completion model
	EmbeddingModel string // embedding model
	BaseURL        string // used by ollama; overrides default OpenAI URL for proxies
	MaxTokens      int    // default completion max tokens (0 = use provider default)
}

// NewFromEnv creates a Provider from environment variables.
// Returns a NoopProvider if no provider is configured — errors on use.
func NewFromEnv() Provider {
	cfg := Config{
		Provider:       envOr("ZERELIO_LLM_PROVIDER", "openai"),
		APIKey:         os.Getenv("ZERELIO_LLM_API_KEY"),
		Model:          envOr("ZERELIO_LLM_MODEL", "gpt-4o-mini"),
		EmbeddingModel: envOr("ZERELIO_EMBEDDING_MODEL", "text-embedding-3-small"),
		BaseURL:        os.Getenv("ZERELIO_LLM_BASE_URL"),
		MaxTokens:      2048,
	}

	switch cfg.Provider {
	case "openai":
		if cfg.APIKey == "" {
			return &NoopProvider{reason: "ZERELIO_LLM_API_KEY not set"}
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
		return newOpenAI(cfg)
	case "anthropic":
		if cfg.APIKey == "" {
			return &NoopProvider{reason: "ZERELIO_LLM_API_KEY not set"}
		}
		return newAnthropic(cfg)
	case "ollama":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434"
		}
		if cfg.EmbeddingModel == "text-embedding-3-small" {
			cfg.EmbeddingModel = "nomic-embed-text" // ollama default
		}
		return newOllama(cfg)
	default:
		return &NoopProvider{reason: "ZERELIO_LLM_PROVIDER not configured"}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
