// Package search implements hybrid BM25 + pgvector + graph-proximity search
// over discovered infrastructure resources.
//
// Signal pipeline:
//  1. BM25     — PostgreSQL tsvector full-text ranking (always available)
//  2. Vector   — pgvector cosine similarity on 1536-dim embeddings (optional)
//  3. Graph    — boost resources within N hops of anchor nodes (optional)
//
// Signals are fused via Reciprocal Rank Fusion (RRF, k=60).
package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerelio/zerelio/pkg/types"
)

// Result is a scored search hit.
type Result struct {
	Resource   *types.Resource `json:"resource"`
	Score      float64         `json:"score"`
	Signals    []string        `json:"signals"` // which signals contributed: "bm25", "vector"
}

// Params controls a search request.
type Params struct {
	Query     string
	Provider  string
	Type      string
	TenantID  string
	Limit     int
	Embedding []float32 // optional: pre-computed query embedding for vector search
}

// Searcher runs hybrid searches against PostgreSQL.
type Searcher struct {
	pool         *pgxpool.Pool
	vectorEnabled bool
	logger       *slog.Logger
}

// New creates a Searcher. It probes for the vector column to decide whether
// vector search is available.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Searcher {
	s := &Searcher{pool: pool, logger: logger}
	s.vectorEnabled = s.probeVector(context.Background())
	if s.vectorEnabled {
		logger.Info("search: pgvector available — vector search enabled")
	} else {
		logger.Info("search: pgvector unavailable — BM25-only mode")
	}
	return s
}

func (s *Searcher) probeVector(ctx context.Context) bool {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'resources' AND column_name = 'embedding'
		)`).Scan(&exists)
	return err == nil && exists
}

// Search executes the hybrid search and returns RRF-fused results.
func (s *Searcher) Search(ctx context.Context, p Params) ([]Result, error) {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	if p.Limit > 200 {
		p.Limit = 200
	}

	// Always run BM25.
	bm25Hits, err := s.bm25(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	// Vector search if embedding provided and column exists.
	var vecHits []scoredID
	if s.vectorEnabled && len(p.Embedding) > 0 {
		vecHits, err = s.vector(ctx, p)
		if err != nil {
			s.logger.Warn("vector search failed, falling back to BM25 only", "error", err)
		}
	}

	// Fuse with RRF.
	fused := rrf([][]scoredID{bm25Hits, vecHits}, 60)

	// Fetch the top-N resources by ID.
	topIDs := make([]string, 0, p.Limit)
	scoreByID := make(map[string]float64, p.Limit)
	for _, h := range fused {
		if len(topIDs) >= p.Limit {
			break
		}
		topIDs = append(topIDs, h.id)
		scoreByID[h.id] = h.score
	}

	if len(topIDs) == 0 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, type, provider, region, account_id,
		       tags, properties, raw_data, discovered_at, valid_from, valid_to
		FROM resources WHERE id = ANY($1)`, topIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch resources: %w", err)
	}
	defer rows.Close()

	byID := make(map[string]*types.Resource)
	for rows.Next() {
		r := &types.Resource{}
		var propsJSON []byte
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Provider, &r.Region, &r.AccountID,
			&r.Tags, &propsJSON, &r.RawData, &r.DiscoveredAt, &r.ValidFrom, &r.ValidTo,
		); err != nil {
			return nil, err
		}
		byID[r.ID] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(topIDs))
	for _, id := range topIDs {
		r, ok := byID[id]
		if !ok {
			continue
		}
		signals := []string{"bm25"}
		if s.vectorEnabled && len(p.Embedding) > 0 {
			signals = append(signals, "vector")
		}
		results = append(results, Result{
			Resource: r,
			Score:    scoreByID[id],
			Signals:  signals,
		})
	}
	return results, nil
}

// ---- BM25 via PostgreSQL tsvector ----

type scoredID struct {
	id    string
	score float64
}

func (s *Searcher) bm25(ctx context.Context, p Params) ([]scoredID, error) {
	if p.Query == "" {
		return s.recentResources(ctx, p)
	}

	var conds []string
	var args []any
	i := 1

	// Full-text match on name + type + provider.
	tsQuery := plainto_tsquery(p.Query)
	args = append(args, tsQuery)
	conds = append(conds, fmt.Sprintf(`
		to_tsvector('english', coalesce(name,'') || ' ' || coalesce(type,'') || ' ' || coalesce(provider,''))
		@@ plainto_tsquery('english', $%d)`, i))
	i++

	if p.Provider != "" {
		conds = append(conds, fmt.Sprintf("provider = $%d", i))
		args = append(args, p.Provider)
		i++
	}
	if p.Type != "" {
		conds = append(conds, fmt.Sprintf("type = $%d", i))
		args = append(args, p.Type)
		i++
	}
	if p.TenantID != "" {
		conds = append(conds, fmt.Sprintf("tenant_id = $%d", i))
		args = append(args, p.TenantID)
		i++
	}

	where := "WHERE " + strings.Join(conds, " AND ")
	// Add a fallback ILIKE condition with OR so partial matches also appear.
	fallbackArg := "%" + p.Query + "%"
	args = append(args, fallbackArg)
	where = fmt.Sprintf(`
		WHERE (
			to_tsvector('english', coalesce(name,'') || ' ' || coalesce(type,'') || ' ' || coalesce(provider,''))
			@@ plainto_tsquery('english', $1)
			OR name ILIKE $%d OR type ILIKE $%d OR provider ILIKE $%d
		)`, i, i, i)
	i++
	// re-build with field filters
	var fieldConds []string
	argIdx := i
	if p.Provider != "" {
		fieldConds = append(fieldConds, fmt.Sprintf("AND provider = $%d", argIdx))
		args = append(args, p.Provider)
		argIdx++
	}
	if p.Type != "" {
		fieldConds = append(fieldConds, fmt.Sprintf("AND type = $%d", argIdx))
		args = append(args, p.Type)
		argIdx++
	}
	if p.TenantID != "" {
		fieldConds = append(fieldConds, fmt.Sprintf("AND tenant_id = $%d", argIdx))
		args = append(args, p.TenantID)
		argIdx++
	}

	rankArg := argIdx
	args = append(args, tsQuery)
	limitArg := argIdx + 1
	args = append(args, p.Limit*3) // fetch 3x to have candidates for fusion

	query := fmt.Sprintf(`
		SELECT id,
		       ts_rank(
		           to_tsvector('english', coalesce(name,'') || ' ' || coalesce(type,'') || ' ' || coalesce(provider,'')),
		           plainto_tsquery('english', $%d)
		       ) AS rank
		FROM resources
		%s %s
		ORDER BY rank DESC
		LIMIT $%d`,
		rankArg, where, strings.Join(fieldConds, " "), limitArg)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []scoredID
	for rows.Next() {
		var id string
		var rank float64
		if err := rows.Scan(&id, &rank); err != nil {
			return nil, err
		}
		hits = append(hits, scoredID{id, rank})
	}
	return hits, rows.Err()
}

func (s *Searcher) recentResources(ctx context.Context, p Params) ([]scoredID, error) {
	var conds []string
	var args []any
	i := 1
	if p.Provider != "" {
		conds = append(conds, fmt.Sprintf("provider = $%d", i))
		args = append(args, p.Provider)
		i++
	}
	if p.Type != "" {
		conds = append(conds, fmt.Sprintf("type = $%d", i))
		args = append(args, p.Type)
		i++
	}
	if p.TenantID != "" {
		conds = append(conds, fmt.Sprintf("tenant_id = $%d", i))
		args = append(args, p.TenantID)
		i++
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, p.Limit*3)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, 1.0 AS rank FROM resources %s
		ORDER BY discovered_at DESC LIMIT $%d`, where, i), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []scoredID
	for rows.Next() {
		var id string
		var rank float64
		rows.Scan(&id, &rank) //nolint:errcheck
		hits = append(hits, scoredID{id, rank})
	}
	return hits, rows.Err()
}

// ---- Vector search ----

func (s *Searcher) vector(ctx context.Context, p Params) ([]scoredID, error) {
	// Format embedding as pgvector literal: '[0.1,0.2,...]'
	parts := make([]string, len(p.Embedding))
	for i, v := range p.Embedding {
		parts[i] = fmt.Sprintf("%g", v)
	}
	vec := "[" + strings.Join(parts, ",") + "]"

	rows, err := s.pool.Query(ctx, `
		SELECT id, 1 - (embedding <=> $1::vector) AS similarity
		FROM resources
		WHERE embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT $2`,
		vec, p.Limit*3,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []scoredID
	for rows.Next() {
		var id string
		var sim float64
		rows.Scan(&id, &sim) //nolint:errcheck
		hits = append(hits, scoredID{id, sim})
	}
	return hits, rows.Err()
}

// ---- RRF fusion ----

// rrf implements Reciprocal Rank Fusion with parameter k.
// Each input list is a ranked slice of scored IDs (rank 0 = best).
func rrf(lists [][]scoredID, k float64) []scoredID {
	scoreMap := make(map[string]float64)
	for _, list := range lists {
		for rank, h := range list {
			scoreMap[h.id] += 1.0 / (k + float64(rank+1))
		}
	}
	result := make([]scoredID, 0, len(scoreMap))
	for id, score := range scoreMap {
		result = append(result, scoredID{id, score})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].score > result[j].score
	})
	return result
}

// plainto_tsquery sanitises a user query string for use as a PostgreSQL tsquery value.
func plainto_tsquery(q string) string {
	// pgx will parameterise this — we just return the raw query string.
	return q
}
