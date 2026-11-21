// Package search implements hybrid BM25 + pgvector + graph-proximity search
// over discovered infrastructure resources.
//
// Signal pipeline:
//  1. BM25     — PostgreSQL tsvector full-text ranking (always available)
//  2. Vector   — pgvector cosine similarity on 1536-dim embeddings (requires
//                LLM provider + populated embeddings column)
//  3. Graph    — boost resources within N hops of top BM25 hits (uses CTE
//                traversal, no AGE required)
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
	"github.com/zerelio/knowledge/internal/llm"
	"github.com/zerelio/knowledge/pkg/types"
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
	pool          *pgxpool.Pool
	llm           llm.Provider // for query embedding; nil = BM25 only
	vectorEnabled bool
	logger        *slog.Logger
}

// New creates a Searcher. It probes for the vector column to decide whether
// vector search is available.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Searcher {
	s := &Searcher{pool: pool, llm: &llm.NoopProvider{}, logger: logger}
	s.vectorEnabled = s.probeVector(context.Background())
	if s.vectorEnabled {
		logger.Info("search: pgvector available — vector search enabled when LLM configured")
	} else {
		logger.Info("search: pgvector unavailable — BM25-only mode")
	}
	return s
}

// SetLLM attaches an LLM provider for query embedding.
// Call this after New() once the LLM provider is configured.
func (s *Searcher) SetLLM(p llm.Provider) {
	s.llm = p
	if !llm.IsNoop(p) && s.vectorEnabled {
		s.logger.Info("search: LLM provider attached — full hybrid search active",
			"provider", p.Name())
	}
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

	// If we have an LLM and vector search is available, embed the query.
	if !llm.IsNoop(s.llm) && s.vectorEnabled && p.Query != "" && len(p.Embedding) == 0 {
		vecs, err := s.llm.Embed(ctx, []string{p.Query})
		if err == nil && len(vecs) > 0 {
			p.Embedding = vecs[0]
		}
		// embed failure is non-fatal — fall back to BM25
	}

	// Always run BM25.
	bm25Hits, err := s.bm25(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	// Vector search if embedding is available.
	var vecHits []scoredID
	if s.vectorEnabled && len(p.Embedding) > 0 {
		vecHits, err = s.vector(ctx, p)
		if err != nil {
			s.logger.Warn("vector search failed, falling back to BM25 only", "error", err)
		}
	}

	// Graph proximity: expand top BM25 results by their graph neighbours.
	var graphHits []scoredID
	if len(bm25Hits) > 0 {
		topID := bm25Hits[0].id
		graphHits, _ = s.graphNeighbours(ctx, topID, 2)
	}

	// Fuse with RRF — fetch more candidates when MMR re-ranking is on.
	fused := rrf([][]scoredID{bm25Hits, vecHits, graphHits}, 60)
	candidateLimit := p.Limit * 3
	if candidateLimit > 60 {
		candidateLimit = 60
	}
	if candidateLimit < p.Limit {
		candidateLimit = p.Limit
	}

	// Collect candidate IDs for the expanded fetch.
	topIDs := make([]string, 0, candidateLimit)
	scoreByID := make(map[string]float64, candidateLimit)
	for _, h := range fused {
		if len(topIDs) >= candidateLimit {
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

	// Build result list in RRF rank order.
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
		if len(graphHits) > 0 {
			signals = append(signals, "graph")
		}
		results = append(results, Result{
			Resource: r,
			Score:    scoreByID[id],
			Signals:  signals,
		})
	}

	// MMR re-ranking: maximize relevance AND diversity.
	// Only kicks in when we fetched more candidates than the requested limit.
	if len(results) > p.Limit {
		results = mmrRerank(results, p.Limit, 0.6)
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

	// $1 = tsQuery (used in WHERE and ts_rank)
	// $2 = ILIKE pattern
	// $3..N = field filters (provider, type, tenant_id)
	// $N+1 = limit
	tsQuery := plainto_tsquery(p.Query)
	ilikePat := "%" + p.Query + "%"
	args := []any{tsQuery, ilikePat}
	i := 3 // next arg index

	var fieldConds []string
	if p.Provider != "" {
		fieldConds = append(fieldConds, fmt.Sprintf("AND provider = $%d", i))
		args = append(args, p.Provider)
		i++
	}
	if p.Type != "" {
		fieldConds = append(fieldConds, fmt.Sprintf("AND type = $%d", i))
		args = append(args, p.Type)
		i++
	}
	if p.TenantID != "" {
		fieldConds = append(fieldConds, fmt.Sprintf("AND tenant_id = $%d", i))
		args = append(args, p.TenantID)
		i++
	}

	limitIdx := i
	args = append(args, p.Limit*3) // fetch 3x candidates for RRF fusion

	query := fmt.Sprintf(`
		SELECT id,
		       ts_rank(
		           to_tsvector('english', coalesce(name,'') || ' ' || coalesce(type,'') || ' ' || coalesce(provider,'')),
		           plainto_tsquery('english', $1)
		       ) AS rank
		FROM resources
		WHERE (
			to_tsvector('english', coalesce(name,'') || ' ' || coalesce(type,'') || ' ' || coalesce(provider,''))
			@@ plainto_tsquery('english', $1)
			OR name ILIKE $2 OR type ILIKE $2 OR provider ILIKE $2
		)
		%s
		ORDER BY rank DESC
		LIMIT $%d`,
		strings.Join(fieldConds, " "), limitIdx)

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

// ---- Graph proximity signal ----

// graphNeighbours returns resources within maxHops of a seed resource.
// These are added as a third signal in RRF to boost topologically related resources.
func (s *Searcher) graphNeighbours(ctx context.Context, seedID string, maxHops int) ([]scoredID, error) {
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE reach AS (
			SELECT source_id AS id, 1 AS depth FROM relationships WHERE source_id = $1 OR target_id = $1
			UNION ALL SELECT target_id AS id, 1 FROM relationships WHERE source_id = $1 OR target_id = $1
			UNION ALL
			SELECT r.target_id, reach.depth + 1
			FROM relationships r JOIN reach ON r.source_id = reach.id
			WHERE reach.depth < $2 AND r.target_id != $1
		)
		SELECT DISTINCT id FROM reach WHERE id != $1`,
		seedID, maxHops)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []scoredID
	rank := 1
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		hits = append(hits, scoredID{id, 1.0 / float64(rank)})
		rank++
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
