// Package embedding populates the resources.embedding column by calling the
// configured LLM provider's Embed API.
//
// A resource's text representation is built from its name, type, provider,
// region, and key properties. This is the text that gets embedded and stored,
// then used at query time to find semantically similar resources.
//
// The Embedder is designed for two use cases:
//  1. Post-discovery batch: called after each discovery run to embed newly
//     discovered resources.
//  2. Background sweep: a ticker that finds un-embedded resources and processes
//     them in batches (catches resources that slipped through).
package embedding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerelio/zerelio/internal/llm"
	"github.com/zerelio/zerelio/pkg/types"
)

const defaultBatchSize = 50

// Embedder writes vector embeddings to the resources table.
type Embedder struct {
	provider  llm.Provider
	pool      *pgxpool.Pool
	batchSize int
	logger    *slog.Logger
}

// New creates an Embedder. If the provider is a noop, all operations are
// silently skipped — the platform runs fine without embeddings.
func New(provider llm.Provider, pool *pgxpool.Pool, logger *slog.Logger) *Embedder {
	return &Embedder{
		provider:  provider,
		pool:      pool,
		batchSize: defaultBatchSize,
		logger:    logger,
	}
}

// EmbedResources embeds the given resources and writes the vectors to the DB.
// Called synchronously after each discovery run.
func (e *Embedder) EmbedResources(ctx context.Context, resources []*types.Resource) {
	if llm.IsNoop(e.provider) || len(resources) == 0 {
		return
	}

	// Process in batches to stay within API rate limits.
	for start := 0; start < len(resources); start += e.batchSize {
		end := start + e.batchSize
		if end > len(resources) {
			end = len(resources)
		}
		batch := resources[start:end]
		if err := e.embedBatch(ctx, batch); err != nil {
			e.logger.Error("embed batch failed", "start", start, "error", err)
		}
	}
}

// SweepUnembedded finds resources with NULL embeddings and embeds them.
// Returns the number of resources processed.
func (e *Embedder) SweepUnembedded(ctx context.Context, limit int) (int, error) {
	if llm.IsNoop(e.provider) {
		return 0, nil
	}
	if limit <= 0 {
		limit = e.batchSize
	}

	rows, err := e.pool.Query(ctx, `
		SELECT id, name, type, provider, region, account_id, properties
		FROM resources WHERE embedding IS NULL
		ORDER BY discovered_at DESC LIMIT $1`, limit)
	if err != nil {
		return 0, fmt.Errorf("sweep query: %w", err)
	}
	defer rows.Close()

	var batch []*types.Resource
	for rows.Next() {
		r := &types.Resource{}
		var propsJSON []byte
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Provider, &r.Region, &r.AccountID, &propsJSON); err != nil {
			return 0, err
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(batch) == 0 {
		return 0, nil
	}

	if err := e.embedBatch(ctx, batch); err != nil {
		return 0, err
	}
	return len(batch), nil
}

// RunBackground starts a ticker that calls SweepUnembedded every interval.
// Stops when ctx is cancelled. Safe to call when provider is noop.
func (e *Embedder) RunBackground(ctx context.Context, interval time.Duration) {
	if llm.IsNoop(e.provider) {
		e.logger.Info("embedding: LLM provider not configured — background sweep disabled")
		return
	}
	e.logger.Info("embedding: background sweep started", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := e.SweepUnembedded(ctx, e.batchSize)
			if err != nil {
				e.logger.Error("embedding sweep error", "error", err)
			} else if n > 0 {
				e.logger.Info("embedding sweep complete", "embedded", n)
			}
		}
	}
}

func (e *Embedder) embedBatch(ctx context.Context, batch []*types.Resource) error {
	texts := make([]string, len(batch))
	for i, r := range batch {
		texts[i] = resourceText(r)
	}

	vectors, err := e.provider.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed API: %w", err)
	}
	if len(vectors) != len(batch) {
		return fmt.Errorf("embed: got %d vectors for %d texts", len(vectors), len(batch))
	}

	for i, r := range batch {
		vec := pgVector(vectors[i])
		if _, err := e.pool.Exec(ctx,
			`UPDATE resources SET embedding = $1 WHERE id = $2`, vec, r.ID); err != nil {
			e.logger.Error("write embedding", "id", r.ID, "error", err)
		}
	}
	return nil
}

// resourceText builds the text string that gets embedded for a resource.
// Keep it dense but human-readable: the embedding model should understand it.
func resourceText(r *types.Resource) string {
	var parts []string
	parts = append(parts, r.Name)
	parts = append(parts, r.Type)
	if r.Provider != "" {
		parts = append(parts, r.Provider)
	}
	if r.Region != "" {
		parts = append(parts, r.Region)
	}
	if r.AccountID != "" {
		parts = append(parts, "account:"+r.AccountID)
	}
	// Add a selection of properties that are semantically meaningful.
	for _, key := range []string{"description", "env", "environment", "cluster", "namespace", "owner", "team"} {
		if v, ok := r.Properties[key]; ok && v != "" {
			parts = append(parts, key+":"+v)
		}
	}
	return strings.Join(parts, " | ")
}

// pgVector formats a []float32 as a pgvector literal string "[0.1,0.2,...]".
func pgVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%g", f))
	}
	sb.WriteByte(']')
	return sb.String()
}
