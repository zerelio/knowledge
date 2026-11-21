package llm

import (
	"context"
	"fmt"
)

// NoopProvider is returned when no LLM provider is configured.
// All operations return an error. The platform starts and runs normally;
// only AI features (search embeddings, doc generation) are unavailable.
type NoopProvider struct {
	reason string
}

func (n *NoopProvider) Name() string { return "noop" }
func (n *NoopProvider) EmbeddingDim() int { return 0 }
func (n *NoopProvider) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("LLM provider not configured (%s)", n.reason)
}
func (n *NoopProvider) Complete(_ context.Context, _ string, _ []Message) (string, error) {
	return "", fmt.Errorf("LLM provider not configured (%s)", n.reason)
}

// IsNoop returns true if the provider is a no-op (not configured).
func IsNoop(p Provider) bool {
	_, ok := p.(*NoopProvider)
	return ok
}
