package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ollamaProvider calls a local Ollama instance.
// Ollama exposes an OpenAI-compatible API at /v1/ so most logic mirrors openAIProvider.
type ollamaProvider struct {
	cfg    Config
	client *http.Client
}

func newOllama(cfg Config) *ollamaProvider {
	return &ollamaProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 300 * time.Second}, // local models can be slow
	}
}

func (p *ollamaProvider) Name() string      { return "ollama" }
func (p *ollamaProvider) EmbeddingDim() int { return ollamaEmbedDim(p.cfg.EmbeddingModel) }

func (p *ollamaProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// Ollama's embeddings endpoint accepts one text at a time (older API).
	// Use the OpenAI-compatible /v1/embeddings when available.
	body, _ := json.Marshal(map[string]any{
		"model": p.cfg.EmbeddingModel,
		"input": texts,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		// Fall back to native /api/embed endpoint.
		return p.embedNative(ctx, texts)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Error != nil {
		return p.embedNative(ctx, texts)
	}
	vectors := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vectors[i] = d.Embedding
	}
	return vectors, nil
}

// embedNative uses Ollama's native /api/embed endpoint.
func (p *ollamaProvider) embedNative(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model": p.cfg.EmbeddingModel,
		"input": texts,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/api/embed", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	return result.Embeddings, nil
}

func (p *ollamaProvider) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	var msgs []map[string]string
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	for _, m := range messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}

	body, _ := json.Marshal(map[string]any{
		"model":    p.cfg.Model,
		"messages": msgs,
		"stream":   false,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama complete: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama complete decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ollama: no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

func ollamaEmbedDim(model string) int {
	switch model {
	case "mxbai-embed-large":
		return 1024
	case "nomic-embed-text":
		return 768
	case "all-minilm":
		return 384
	default:
		return 768 // common default
	}
}
