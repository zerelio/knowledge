package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// anthropicProvider calls Anthropic's Messages API.
type anthropicProvider struct {
	cfg    Config
	client *http.Client
}

func newAnthropic(cfg Config) *anthropicProvider {
	return &anthropicProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *anthropicProvider) Name() string { return "anthropic" }

// Anthropic does not expose an embeddings API — return an error.
func (p *anthropicProvider) EmbeddingDim() int { return 0 }
func (p *anthropicProvider) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("anthropic does not provide an embeddings API; set ZERELIO_LLM_PROVIDER=openai for embeddings")
}

func (p *anthropicProvider) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	var msgs []map[string]string
	for _, m := range messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}

	payload := map[string]any{
		"model":      p.cfg.Model,
		"max_tokens": p.cfg.MaxTokens,
		"messages":   msgs,
	}
	if system != "" {
		payload["system"] = system
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic complete: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("anthropic complete decode: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("anthropic error: %s", result.Error.Message)
	}
	for _, c := range result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text content in response")
}
