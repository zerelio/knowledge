package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerelio/knowledge/internal/auth"
	"github.com/zerelio/knowledge/internal/discovery"
	"github.com/zerelio/knowledge/internal/docs"
	"github.com/zerelio/knowledge/internal/events"
	"github.com/zerelio/knowledge/internal/graph"
	"github.com/zerelio/knowledge/internal/oidcauth"
	"github.com/zerelio/knowledge/internal/search"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

type Handler struct {
	store    storage.Backend
	jwt      *auth.JWTService
	orch     *discovery.Orchestrator
	graph    *graph.Graph          // optional
	searcher *search.Searcher      // optional
	docGen   *docs.Generator       // optional
	bus      *events.Bus           // optional
	oidc     *oidcauth.Provider    // optional
	oidcCfg  *oidcauth.Config      // optional
	scheduler interface{ IsRunning() bool } // optional — for health check
	logger   *slog.Logger
}

func NewHandler(store storage.Backend, jwt *auth.JWTService, orch *discovery.Orchestrator, logger *slog.Logger) *Handler {
	return &Handler{
		store:  store,
		jwt:    jwt,
		orch:   orch,
		logger: logger,
	}
}

// SetGraph attaches the graph module to the handler (called after setup in main).
func (h *Handler) SetGraph(g *graph.Graph) {
	h.graph = g
}

// SetSearcher attaches the search module to the handler.
func (h *Handler) SetSearcher(s *search.Searcher) {
	h.searcher = s
}

// SetDocGenerator attaches the doc generation module.
func (h *Handler) SetDocGenerator(g *docs.Generator) {
	h.docGen = g
}

// SetOIDC attaches the OIDC provider for SSO.
func (h *Handler) SetOIDC(p *oidcauth.Provider, cfg *oidcauth.Config) {
	h.oidc = p
	h.oidcCfg = cfg
}

// SetScheduler attaches the scheduler reference for health reporting.
// The scheduler is expected to implement IsRunning() bool.
func (h *Handler) SetScheduler(s interface{ IsRunning() bool }) {
	h.scheduler = s
}

// SetEventBus attaches the event bus to the handler.
func (h *Handler) SetEventBus(b *events.Bus) {
	h.bus = b
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	pair, err := h.jwt.IssueTokens(user.ID, user.Email, user.TenantID, user.Roles)
	if err != nil {
		h.logger.Error("issue tokens", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	if err := h.store.StoreRefreshToken(r.Context(), pair.RefreshJTI, user.ID, pair.RefreshTokenExpiry); err != nil {
		h.logger.Error("store refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store session")
		return
	}

	writeJSON(w, http.StatusOK, pair)
}

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req types.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(req.Password) < 12 {
		writeError(w, http.StatusBadRequest, "password must be at least 12 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	roles := req.Roles
	if len(roles) == 0 {
		roles = []string{types.RoleViewer}
	}

	now := time.Now()
	user := &types.User{
		ID:           fmt.Sprintf("usr_%d", now.UnixNano()),
		Email:        req.Email,
		PasswordHash: hash,
		Roles:        roles,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.store.CreateUser(r.Context(), user); err != nil {
		writeError(w, http.StatusConflict, "user already exists")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":    user.ID,
		"email": user.Email,
	})
}

func (h *Handler) HandleListResources(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())

	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	filter := storage.ResourceFilter{
		Type:     r.URL.Query().Get("type"),
		Provider: r.URL.Query().Get("provider"),
		Region:   r.URL.Query().Get("region"),
		Query:    r.URL.Query().Get("q"),
		TenantID: claims.TenantID,
		Limit:    limit,
	}

	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		t, id, err := decodeCursor(cursorStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		filter.AfterDiscoveredAt = &t
		filter.AfterID = id
	} else if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	resources, total, err := h.store.GetResources(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list resources")
		return
	}

	var nextCursor string
	hasMore := false
	if len(resources) == limit && limit > 0 {
		hasMore = true
		nextCursor = encodeCursor(resources[len(resources)-1])
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"resources":   resources,
		"total":       total,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

// cursorPayload is the JSON structure embedded in a base64 cursor token.
type cursorPayload struct {
	AfterDiscoveredAt time.Time `json:"after_discovered_at"`
	AfterID           string    `json:"after_id"`
}

// encodeCursor returns a base64-encoded JSON cursor pointing just after r.
func encodeCursor(r *types.Resource) string {
	p := cursorPayload{AfterDiscoveredAt: r.DiscoveredAt, AfterID: r.ID}
	b, _ := json.Marshal(p)
	return base64.StdEncoding.EncodeToString(b)
}

// decodeCursor decodes a cursor token produced by encodeCursor.
func decodeCursor(s string) (time.Time, string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", err
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return time.Time{}, "", err
	}
	return p.AfterDiscoveredAt, p.AfterID, nil
}

func (h *Handler) HandleGetResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resource, err := h.store.GetResourceByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

func (h *Handler) HandleCreateResources(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Resources []*types.Resource `json:"resources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	for _, res := range req.Resources {
		res.TenantID = claims.TenantID
		if res.DiscoveredAt.IsZero() {
			res.DiscoveredAt = time.Now()
		}
	}

	if err := h.store.StoreResources(r.Context(), req.Resources); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store resources")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int{"stored": len(req.Resources)})
}

func (h *Handler) HandleDeleteResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteResources(r.Context(), []string{id}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete resource")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleGraphExport(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	resources, rels, err := h.store.ExportGraph(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export graph")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": resources,
		"edges": rels,
	})
}

func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	users, err := h.store.ListUsers(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req types.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	roles := req.Roles
	if len(roles) == 0 {
		roles = []string{types.RoleViewer}
	}

	now := time.Now()
	user := &types.User{
		ID:           fmt.Sprintf("usr_%d", now.UnixNano()),
		Email:        req.Email,
		PasswordHash: hash,
		Roles:        roles,
		TenantID:     claims.TenantID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.store.CreateUser(r.Context(), user); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": user.ID, "email": user.Email})
}

func (h *Handler) HandleListScopes(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	scopes, err := h.store.ListScopes(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list scopes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scopes": scopes})
}

func (h *Handler) HandleCreateScope(w http.ResponseWriter, r *http.Request) {
	var req types.DiscoveryScope
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.PluginName == "" {
		writeError(w, http.StatusBadRequest, "name and plugin_name are required")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	req.ID = fmt.Sprintf("scope_%d", time.Now().UnixNano())
	req.TenantID = claims.TenantID

	if err := h.store.CreateScope(r.Context(), &req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create scope")
		return
	}
	writeJSON(w, http.StatusCreated, &req)
}

func (h *Handler) HandleGetScope(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scope, err := h.store.GetScope(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "scope not found")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if scope.TenantID != claims.TenantID && !containsRole(claims.Roles, "admin") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	writeJSON(w, http.StatusOK, scope)
}

func (h *Handler) HandleUpdateScope(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	scope, err := h.store.GetScope(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "scope not found")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if scope.TenantID != claims.TenantID && !containsRole(claims.Roles, "admin") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var patch struct {
		Name     *string           `json:"name"`
		Schedule *string           `json:"schedule"`
		Config   map[string]string `json:"config"`
		Status   *string           `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if patch.Name != nil {
		scope.Name = *patch.Name
	}
	if patch.Schedule != nil {
		scope.Schedule = *patch.Schedule
	}
	if patch.Config != nil {
		scope.Config = patch.Config
	}
	if patch.Status != nil {
		scope.Status = *patch.Status
	}
	scope.UpdatedAt = time.Now()

	if err := h.store.UpdateScope(r.Context(), scope); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update scope")
		return
	}
	writeJSON(w, http.StatusOK, scope)
}

func (h *Handler) HandleDeleteScope(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	scope, err := h.store.GetScope(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "scope not found")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if scope.TenantID != claims.TenantID && !containsRole(claims.Roles, "admin") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := h.store.DeleteScope(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete scope")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleRunScope(w http.ResponseWriter, r *http.Request) {
	if h.orch == nil {
		writeError(w, http.StatusServiceUnavailable, "discovery not configured")
		return
	}

	scopeID := chi.URLParam(r, "id")
	run, err := h.orch.RunScope(r.Context(), scopeID)
	if err != nil {
		h.logger.Error("run scope", "scope_id", scopeID, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

// HandleListRuns returns recent discovery runs for a scope.
// GET /api/v1/discovery/scopes/{id}/runs?limit=50
func (h *Handler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")
	runs, err := h.store.ListRuns(r.Context(), scopeID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// HandleListAllRuns returns recent discovery runs across all scopes.
// GET /api/v1/discovery/runs?limit=50
func (h *Handler) HandleListAllRuns(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	runs, err := h.store.ListRuns(r.Context(), "", limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// HandleGetRun returns a single discovery run by its ID.
// GET /api/v1/discovery/runs/{runID}
func (h *Handler) HandleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := h.store.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// HandleListChanges returns resource changes detected for a scope.
// GET /api/v1/discovery/scopes/{id}/changes?limit=50
func (h *Handler) HandleListChanges(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")
	claims := auth.ClaimsFromContext(r.Context())
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	changes, err := h.store.ListChanges(r.Context(), scopeID, claims.TenantID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list changes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes, "total": len(changes)})
}

func (h *Handler) HandleListPlugins(w http.ResponseWriter, r *http.Request) {
	var plugins []string
	if h.orch != nil {
		plugins = h.orch.ListPlugins()
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": plugins})
}

func (h *Handler) HandleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	// Admins can query cross-tenant; others see only their own tenant.
	tenantID := claims.TenantID
	if r.URL.Query().Get("tenant_id") != "" && containsRole(claims.Roles, "admin") {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	events, err := h.store.ListAuditEvents(r.Context(), tenantID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "total": len(events)})
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// ---- Docs + runbook handlers ----

// HandleGenerateDoc generates an LLM-powered dossier for a resource.
// GET /api/v1/docs/{id}
func (h *Handler) HandleGenerateDoc(w http.ResponseWriter, r *http.Request) {
	if h.docGen == nil {
		writeError(w, http.StatusServiceUnavailable,
			"documentation generation requires an LLM provider (set ZERELIO_LLM_PROVIDER)")
		return
	}
	id := chi.URLParam(r, "id")
	doc, err := h.docGen.GenerateResourceDoc(r.Context(), id)
	if err != nil {
		h.logger.Error("generate doc", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "doc generation failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// HandleGenerateRunbook generates an LLM-powered runbook for a scenario.
// POST /api/v1/runbooks
// Body: {"scenario": "...", "resource_ids": ["res_001", ...]}
func (h *Handler) HandleGenerateRunbook(w http.ResponseWriter, r *http.Request) {
	if h.docGen == nil {
		writeError(w, http.StatusServiceUnavailable,
			"runbook generation requires an LLM provider (set ZERELIO_LLM_PROVIDER)")
		return
	}
	var req struct {
		Scenario    string   `json:"scenario"`
		ResourceIDs []string `json:"resource_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Scenario == "" {
		writeError(w, http.StatusBadRequest, "scenario required")
		return
	}
	rb, err := h.docGen.GenerateRunbook(r.Context(), req.Scenario, req.ResourceIDs)
	if err != nil {
		h.logger.Error("generate runbook", "scenario", req.Scenario, "error", err)
		writeError(w, http.StatusInternalServerError, "runbook generation failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rb)
}

// ---- Generation feedback + quality ----

// HandleSubmitFeedback records human feedback on a generation.
// POST /api/v1/generations/{id}/feedback
func (h *Handler) HandleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	genID := chi.URLParam(r, "id")
	var req types.GenerationFeedback
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	req.ID = fmt.Sprintf("fb_%d", time.Now().UnixNano())
	req.GenerationID = genID
	req.UserID = claims.UserID
	req.TenantID = claims.TenantID
	req.CreatedAt = time.Now()

	if err := h.store.LogGenerationFeedback(r.Context(), &req); err != nil {
		h.logger.Error("log generation feedback", "gen_id", genID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save feedback")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": req.ID, "generation_id": genID})
}

// HandleGetGeneration returns a generation log with its full context and feedback.
// GET /api/v1/generations/{id}
func (h *Handler) HandleGetGeneration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gen, err := h.store.GetGeneration(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "generation not found")
		return
	}
	feedback, _ := h.store.ListGenerationFeedback(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{
		"generation": gen,
		"feedback":   feedback,
	})
}

// HandleListGenerations lists generation logs for a tenant.
// GET /api/v1/generations?resource_id=...&limit=50
func (h *Handler) HandleListGenerations(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	resourceID := r.URL.Query().Get("resource_id")
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	gens, err := h.store.ListGenerations(r.Context(), claims.TenantID, resourceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list generations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generations": gens, "total": len(gens)})
}

// HandleDocumentStatus updates a document's review status.
// PUT /api/v1/generations/{id}/status
// Body: {"status": "approved"|"rejected"|"flagged"}
func (h *Handler) HandleDocumentStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
		writeError(w, http.StatusBadRequest, "status required")
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	status := types.DocumentStatus(req.Status)
	if err := h.store.UpdateDocumentStatus(r.Context(), id, status, claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": req.Status})
}

// HandleQualityMetrics returns aggregated quality metrics for AI generations.
// GET /api/v1/admin/quality
func (h *Handler) HandleQualityMetrics(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	metrics, err := h.store.GetQualityMetrics(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get quality metrics")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// ---- User management ----

// HandleGetUser returns a single user by ID (admin only).
// GET /api/v1/admin/users/{id}
func (h *Handler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.store.GetUserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	user.PasswordHash = "" // never expose
	writeJSON(w, http.StatusOK, user)
}

// HandleUpdateUser partially updates a user's email or roles (admin only).
// PATCH /api/v1/admin/users/{id}
// Body: {"email": "...", "roles": [...]}
func (h *Handler) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.store.GetUserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var req struct {
		Email *string  `json:"email,omitempty"`
		Roles []string `json:"roles,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if len(req.Roles) > 0 {
		user.Roles = req.Roles
	}
	if err := h.store.UpdateUser(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	user.PasswordHash = ""
	writeJSON(w, http.StatusOK, user)
}

// HandleDeleteUser deletes a user by ID (admin only).
// DELETE /api/v1/admin/users/{id}
func (h *Handler) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Prevent self-deletion.
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil && claims.UserID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	if err := h.store.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---- Graph handlers ----

// HandleSearch runs a hybrid BM25 + vector search over resources.
// GET /api/v1/search?q=...&provider=...&type=...&limit=20
func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	provider := r.URL.Query().Get("provider")
	resType := r.URL.Query().Get("type")
	limit := 20
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	claims := auth.ClaimsFromContext(r.Context())

	// Use the full-featured searcher when available (PG backend).
	if h.searcher != nil {
		results, err := h.searcher.Search(r.Context(), search.Params{
			Query:    q,
			Provider: provider,
			Type:     resType,
			TenantID: claims.TenantID,
			Limit:    limit,
		})
		if err != nil {
			h.logger.Error("search", "query", q, "error", err)
			writeError(w, http.StatusInternalServerError, "search failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"results": results,
			"total":   len(results),
			"query":   q,
		})
		return
	}

	// Fallback: use storage filter (MemoryBackend or no searcher).
	resources, total, err := h.store.GetResources(r.Context(), storage.ResourceFilter{
		Query:    q,
		Provider: provider,
		Type:     resType,
		TenantID: claims.TenantID,
		Limit:    limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	results := make([]map[string]any, len(resources))
	for i, res := range resources {
		results[i] = map[string]any{"resource": res, "score": 1.0, "signals": []string{"filter"}}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   total,
		"query":   q,
	})
}

// ---- Graph handlers ----

// HandleNeighborhood returns all nodes and edges within N hops of a resource.
// GET /api/v1/graph/neighborhood/{id}?hops=2
func (h *Handler) HandleNeighborhood(w http.ResponseWriter, r *http.Request) {
	if h.graph == nil {
		writeError(w, http.StatusServiceUnavailable, "graph module not available (requires PostgreSQL backend)")
		return
	}
	id := chi.URLParam(r, "id")
	hops := 2
	if s := r.URL.Query().Get("hops"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			hops = n
		}
	}

	subgraph, err := h.graph.Neighborhood(r.Context(), id, hops)
	if err != nil {
		h.logger.Error("neighborhood query", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "graph query failed")
		return
	}
	writeJSON(w, http.StatusOK, subgraph)
}

// HandleCypher executes a raw Cypher query against the AGE graph.
// POST /api/v1/graph/cypher
// Body: {"query": "MATCH (n:Resource) RETURN n", "return": ["n"]}
func (h *Handler) HandleCypher(w http.ResponseWriter, r *http.Request) {
	if h.graph == nil {
		writeError(w, http.StatusServiceUnavailable, "graph module not available (requires PostgreSQL backend)")
		return
	}

	var req struct {
		Query      string   `json:"query"`
		ReturnCols []string `json:"return"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}

	results, err := h.graph.Cypher(r.Context(), req.Query, req.ReturnCols)
	if err != nil {
		h.logger.Error("cypher query", "error", err)
		writeError(w, http.StatusInternalServerError, "cypher query failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// HandleRefresh validates a refresh token, revokes it, and issues a new token pair.
// This implements single-use refresh token rotation: each use produces a new pair.
func (h *Handler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	// Validate signature + expiry.
	claims, err := h.jwt.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Look up JTI in store — must exist and not be revoked.
	rt, err := h.store.GetRefreshToken(r.Context(), claims.ID)
	if err != nil || rt.Revoked || rt.ExpiresAt.Before(time.Now()) {
		writeError(w, http.StatusUnauthorized, "refresh token revoked or expired")
		return
	}

	// Revoke the used JTI (rotation: one-time use).
	if err := h.store.RevokeRefreshToken(r.Context(), claims.ID); err != nil {
		h.logger.Error("revoke refresh token", "jti", claims.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to rotate token")
		return
	}

	// Load the user to get current roles and tenant.
	user, err := h.store.GetUserByID(r.Context(), claims.Subject)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	// Issue new token pair and store the new JTI.
	pair, err := h.jwt.IssueTokens(user.ID, user.Email, user.TenantID, user.Roles)
	if err != nil {
		h.logger.Error("issue tokens on refresh", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	if err := h.store.StoreRefreshToken(r.Context(), pair.RefreshJTI, user.ID, pair.RefreshTokenExpiry); err != nil {
		h.logger.Error("store new refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store session")
		return
	}

	writeJSON(w, http.StatusOK, pair)
}

// ---- Webhook handlers ----

// HandleCreateWebhook creates a new webhook subscription.
// POST /api/v1/webhooks
func (h *Handler) HandleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req types.WebhookSubscription
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" || req.Secret == "" {
		writeError(w, http.StatusBadRequest, "url and secret are required")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	req.ID = fmt.Sprintf("wh_%d", time.Now().UnixNano())
	req.TenantID = claims.TenantID
	req.Active = true
	req.CreatedAt = time.Now()

	if err := h.store.CreateWebhook(r.Context(), &req); err != nil {
		h.logger.Error("create webhook", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}

	// Return without the secret.
	resp := req
	resp.Secret = ""
	writeJSON(w, http.StatusCreated, &resp)
}

// HandleListWebhooks lists webhook subscriptions for the current tenant.
// GET /api/v1/webhooks
func (h *Handler) HandleListWebhooks(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	subs, err := h.store.ListWebhooks(r.Context(), claims.TenantID)
	if err != nil {
		h.logger.Error("list webhooks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list webhooks")
		return
	}
	// Omit secrets from the response.
	for _, s := range subs {
		s.Secret = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": subs})
}

// HandleDeleteWebhook deletes a webhook subscription.
// DELETE /api/v1/webhooks/{id}
func (h *Handler) HandleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteWebhook(r.Context(), id); err != nil {
		h.logger.Error("delete webhook", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
