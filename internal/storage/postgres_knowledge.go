package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zerelio/zerelio/pkg/types"
)

// ---- Generation logs ----

func (p *PGBackend) LogGeneration(ctx context.Context, log *types.GenerationLog) error {
	retrievedJSON, _ := json.Marshal(log.RetrievedIDs)
	signalsJSON, _ := json.Marshal(log.RetrievalSignals)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO generation_logs
			(id, type, tenant_id, user_id, resource_id, query, context_text,
			 retrieved_ids, retrieval_signals, model, provider,
			 tokens_prompt, tokens_completion, latency_ms, output, auto_score, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (id) DO NOTHING`,
		log.ID, string(log.Type), log.TenantID, log.UserID, log.ResourceID,
		log.Query, log.ContextText, retrievedJSON, signalsJSON,
		log.Model, log.Provider,
		log.TokensPrompt, log.TokensCompletion, log.LatencyMS,
		log.Output, log.AutoScore, log.CreatedAt,
	)
	return err
}

func (p *PGBackend) GetGeneration(ctx context.Context, id string) (*types.GenerationLog, error) {
	log := &types.GenerationLog{}
	var retrievedJSON, signalsJSON []byte
	err := p.pool.QueryRow(ctx, `
		SELECT id, type, tenant_id, user_id, resource_id, query, context_text,
		       retrieved_ids, retrieval_signals, model, provider,
		       tokens_prompt, tokens_completion, latency_ms, output, auto_score, created_at
		FROM generation_logs WHERE id = $1`, id).Scan(
		&log.ID, &log.Type, &log.TenantID, &log.UserID, &log.ResourceID,
		&log.Query, &log.ContextText,
		&retrievedJSON, &signalsJSON,
		&log.Model, &log.Provider,
		&log.TokensPrompt, &log.TokensCompletion, &log.LatencyMS,
		&log.Output, &log.AutoScore, &log.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get generation %s: %w", id, err)
	}
	json.Unmarshal(retrievedJSON, &log.RetrievedIDs)       //nolint:errcheck
	json.Unmarshal(signalsJSON, &log.RetrievalSignals)     //nolint:errcheck
	return log, nil
}

func (p *PGBackend) ListGenerations(ctx context.Context, tenantID, resourceID string, limit int) ([]*types.GenerationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var conds []string
	var args []any
	i := 1
	if tenantID != "" {
		conds = append(conds, fmt.Sprintf("tenant_id = $%d", i))
		args = append(args, tenantID)
		i++
	}
	if resourceID != "" {
		conds = append(conds, fmt.Sprintf("resource_id = $%d", i))
		args = append(args, resourceID)
		i++
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit)

	rows, err := p.pool.Query(ctx, `
		SELECT id, type, tenant_id, user_id, resource_id, query,
		       model, provider, tokens_prompt, tokens_completion, latency_ms, auto_score, created_at
		FROM generation_logs `+where+` ORDER BY created_at DESC LIMIT $`+fmt.Sprintf("%d", i),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*types.GenerationLog
	for rows.Next() {
		log := &types.GenerationLog{}
		if err := rows.Scan(&log.ID, &log.Type, &log.TenantID, &log.UserID, &log.ResourceID,
			&log.Query, &log.Model, &log.Provider, &log.TokensPrompt, &log.TokensCompletion,
			&log.LatencyMS, &log.AutoScore, &log.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, log)
	}
	return result, rows.Err()
}

// ---- Feedback ----

func (p *PGBackend) LogGenerationFeedback(ctx context.Context, fb *types.GenerationFeedback) error {
	missingJSON, _ := json.Marshal(fb.MissingResourceIDs)
	spuriousJSON, _ := json.Marshal(fb.SpuriousResourceIDs)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO generation_feedback
			(id, generation_id, user_id, tenant_id, rating, anomaly_type, comment,
			 corrected_output, missing_resource_ids, spurious_resource_ids, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		fb.ID, fb.GenerationID, fb.UserID, fb.TenantID,
		fb.Rating, string(fb.AnomalyType), fb.Comment,
		fb.CorrectedOutput, missingJSON, spuriousJSON, fb.CreatedAt,
	)
	return err
}

func (p *PGBackend) ListGenerationFeedback(ctx context.Context, generationID string) ([]*types.GenerationFeedback, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, generation_id, user_id, tenant_id, rating, anomaly_type, comment,
		       corrected_output, missing_resource_ids, spurious_resource_ids, created_at
		FROM generation_feedback WHERE generation_id = $1 ORDER BY created_at`, generationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*types.GenerationFeedback
	for rows.Next() {
		fb := &types.GenerationFeedback{}
		var missingJSON, spuriousJSON []byte
		if err := rows.Scan(&fb.ID, &fb.GenerationID, &fb.UserID, &fb.TenantID,
			&fb.Rating, &fb.AnomalyType, &fb.Comment,
			&fb.CorrectedOutput, &missingJSON, &spuriousJSON, &fb.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(missingJSON, &fb.MissingResourceIDs)   //nolint:errcheck
		json.Unmarshal(spuriousJSON, &fb.SpuriousResourceIDs) //nolint:errcheck
		result = append(result, fb)
	}
	return result, rows.Err()
}

func (p *PGBackend) GetApprovedExamples(ctx context.Context, genType types.GenerationType, hint string, limit int) ([]*types.GenerationLog, error) {
	if limit <= 0 {
		limit = 3
	}
	// Find generations of this type that have at least one positive rating.
	// Optionally filter by resource_id LIKE hint for type-aware few-shot.
	rows, err := p.pool.Query(ctx, `
		SELECT DISTINCT ON (gl.id) gl.id, gl.context_text, gl.output, gl.model, gl.provider, gl.created_at
		FROM generation_logs gl
		JOIN generation_feedback gf ON gf.generation_id = gl.id
		WHERE gl.type = $1 AND gf.rating > 0
		ORDER BY gl.id, gl.created_at DESC
		LIMIT $2`,
		string(genType), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*types.GenerationLog
	for rows.Next() {
		log := &types.GenerationLog{}
		if err := rows.Scan(&log.ID, &log.ContextText, &log.Output, &log.Model, &log.Provider, &log.CreatedAt); err != nil {
			return nil, err
		}
		log.Type = genType
		result = append(result, log)
	}
	return result, rows.Err()
}

func (p *PGBackend) GetQualityMetrics(ctx context.Context, tenantID string) (*types.QualityMetrics, error) {
	tenantCond := ""
	var args []any
	if tenantID != "" {
		tenantCond = "WHERE tenant_id = $1"
		args = append(args, tenantID)
	}

	metrics := &types.QualityMetrics{ByType: make(map[string]int), ByAnomaly: make(map[string]int)}

	// Total counts by type.
	rows, err := p.pool.Query(ctx, `SELECT type, COUNT(*) FROM generation_logs `+tenantCond+` GROUP BY type`, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t string
			var n int
			rows.Scan(&t, &n) //nolint:errcheck
			metrics.TotalGenerations += n
			metrics.ByType[t] = n
		}
	}

	// Feedback aggregates.
	fbRows, err := p.pool.Query(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE rating > 0)::float / NULLIF(COUNT(*),0),
			COUNT(*) FILTER (WHERE rating < 0)::float / NULLIF(COUNT(*),0),
			COUNT(*) FILTER (WHERE anomaly_type = 'hallucination')::float / NULLIF(COUNT(*),0),
			COUNT(*)
		FROM generation_feedback`)
	if err == nil {
		defer fbRows.Close()
		if fbRows.Next() {
			fbRows.Scan(&metrics.ApprovalRate, &metrics.RejectionRate, &metrics.HallucinationRate, &metrics.WithFeedback) //nolint:errcheck
		}
	}

	// Anomaly breakdown.
	anomalyRows, err := p.pool.Query(ctx, `SELECT anomaly_type, COUNT(*) FROM generation_feedback WHERE anomaly_type != '' GROUP BY anomaly_type`)
	if err == nil {
		defer anomalyRows.Close()
		for anomalyRows.Next() {
			var t string
			var n int
			anomalyRows.Scan(&t, &n) //nolint:errcheck
			metrics.ByAnomaly[t] = n
		}
	}

	return metrics, nil
}

// ---- Document cache ----

func (p *PGBackend) UpsertDocument(ctx context.Context, doc *types.CachedDocument) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO documents (id, resource_id, title, content, generation_id, status, auto_score, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET
			content=EXCLUDED.content, status=EXCLUDED.status, auto_score=EXCLUDED.auto_score,
			updated_at=now()`,
		doc.ID, doc.ResourceID, doc.Title, doc.Content, doc.GenerationID,
		string(doc.Status), doc.AutoScore, doc.CreatedAt, doc.UpdatedAt,
	)
	return err
}

func (p *PGBackend) GetDocument(ctx context.Context, resourceID string) (*types.CachedDocument, error) {
	doc := &types.CachedDocument{}
	err := p.pool.QueryRow(ctx, `
		SELECT id, resource_id, title, content, generation_id, status, auto_score,
		       approved_by, approved_at, created_at, updated_at
		FROM documents WHERE resource_id = $1
		ORDER BY created_at DESC LIMIT 1`, resourceID).Scan(
		&doc.ID, &doc.ResourceID, &doc.Title, &doc.Content, &doc.GenerationID,
		&doc.Status, &doc.AutoScore,
		&doc.ApprovedBy, &doc.ApprovedAt, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get document for %s: %w", resourceID, err)
	}
	return doc, nil
}

func (p *PGBackend) UpdateDocumentStatus(ctx context.Context, id string, status types.DocumentStatus, approvedBy string) error {
	var approvedAt *time.Time
	if status == types.DocumentApproved {
		now := time.Now()
		approvedAt = &now
	}
	_, err := p.pool.Exec(ctx, `
		UPDATE documents SET status=$2, approved_by=$3, approved_at=$4, updated_at=now()
		WHERE id=$1`,
		id, string(status), approvedBy, approvedAt,
	)
	return err
}
