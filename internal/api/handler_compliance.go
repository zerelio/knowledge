package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerelio/zerelio/internal/auth"
	"github.com/zerelio/zerelio/pkg/types"
)

// HandleListKnowledgeSummaries returns pre-computed community-level summaries.
// GET /api/v1/knowledge/summaries?scope_id=...
func (h *Handler) HandleListKnowledgeSummaries(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	scopeID := r.URL.Query().Get("scope_id")
	summaries, err := h.store.ListKnowledgeSummaries(r.Context(), claims.TenantID, scopeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list summaries")
		return
	}
	if summaries == nil {
		summaries = []*types.KnowledgeSummary{}
	}
	writeJSON(w, http.StatusOK, summaries)
}

// HandleListFindings returns compliance findings for the authenticated tenant.
// GET /api/v1/compliance/findings?scope_id=...&severity=critical&status=open&limit=100
func (h *Handler) HandleListFindings(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	q := r.URL.Query()

	scopeID := q.Get("scope_id")
	severity := types.FindingSeverity(q.Get("severity"))
	status := types.FindingStatus(q.Get("status"))
	limit := 100
	if l := q.Get("limit"); l != "" {
		if n, err := fmt.Sscanf(l, "%d", &limit); n == 0 || err != nil || limit < 1 || limit > 1000 {
			limit = 100
		}
	}

	findings, err := h.store.ListFindings(r.Context(), claims.TenantID, scopeID, severity, status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list findings")
		return
	}
	if findings == nil {
		findings = []*types.ComplianceFinding{}
	}
	writeJSON(w, http.StatusOK, findings)
}

// HandleUpdateFindingStatus sets a finding's status (suppress or resolve).
// PATCH /api/v1/compliance/findings/{id}
// Body: {"status": "suppressed"|"resolved"|"open"}
func (h *Handler) HandleUpdateFindingStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Status types.FindingStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch req.Status {
	case types.FindingStatusOpen, types.FindingStatusSuppressed, types.FindingStatusResolved:
		// valid
	default:
		writeError(w, http.StatusBadRequest, "status must be one of: open, suppressed, resolved")
		return
	}
	if err := h.store.UpdateFindingStatus(r.Context(), id, req.Status); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update finding status")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleHealth returns the system health status.
// GET /api/v1/health  (unauthenticated — used by load balancers and the console dashboard)
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	type componentStatus struct {
		Status  string `json:"status"`
		Message string `json:"message,omitempty"`
	}
	type healthResponse struct {
		Status     string                     `json:"status"`
		Version    string                     `json:"version"`
		Time       time.Time                  `json:"time"`
		Components map[string]componentStatus `json:"components"`
	}

	components := map[string]componentStatus{}
	overall := "ok"

	// DB ping.
	if err := h.store.Ping(r.Context()); err != nil {
		components["db"] = componentStatus{Status: "error", Message: err.Error()}
		overall = "degraded"
	} else {
		components["db"] = componentStatus{Status: "ok"}
	}

	// Scheduler.
	if h.scheduler != nil {
		components["scheduler"] = componentStatus{Status: "ok"}
	} else {
		components["scheduler"] = componentStatus{Status: "disabled", Message: "set ZERELIO_SCHEDULER_ENABLED=true"}
	}

	// OIDC.
	if h.oidc != nil {
		components["oidc"] = componentStatus{Status: "ok"}
	} else {
		components["oidc"] = componentStatus{Status: "disabled", Message: "set ZERELIO_OIDC_ISSUER to enable SSO"}
	}

	// Orchestrator (always present).
	components["orchestrator"] = componentStatus{Status: "ok"}

	// MCP.
	components["mcp"] = componentStatus{Status: "ok"}

	status := http.StatusOK
	if overall == "degraded" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, healthResponse{
		Status:     overall,
		Version:    "0.1.0",
		Time:       time.Now(),
		Components: components,
	})
}
