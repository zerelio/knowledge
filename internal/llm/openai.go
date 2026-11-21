package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// openAIProvider implements Provider via the OpenAI REST API.
// Also compatible with Azure OpenAI and any OpenAI-spec proxy.
type openAIProvider struct {
	cfg    Config
	client *http.Client
}

func newOpenAI(cfg Config) *openAIProvider {
	return &openAIProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *openAIProvider) Name() string         { return "openai" }
func (p *openAIProvider) EmbeddingDim() int    { return openAIEmbedDim(p.cfg.EmbeddingModel) }

func (p *openAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model": p.cfg.EmbeddingModel,
		"input": texts,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai embed decode: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openai embed error: %s", result.Error.Message)
	}
	vectors := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vectors[i] = d.Embedding
	}
	return vectors, nil
}

func (p *openAIProvider) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	var msgs []map[string]string
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	for _, m := range messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}

	body, _ := json.Marshal(map[string]any{
		"model":      p.cfg.Model,
		"messages":   msgs,
		"max_tokens": p.cfg.MaxTokens,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai complete: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("openai complete decode: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("openai complete error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai complete: no choices returned")
	}
	return result.Choices[0].Message.Content, nil
}

func openAIEmbedDim(model string) int {
	switch model {
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	default: // text-embedding-3-small and future defaults
		return 1536
	}
}
