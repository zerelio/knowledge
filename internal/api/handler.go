package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerelio/zerelio/internal/auth"
	"github.com/zerelio/zerelio/internal/discovery"
	"github.com/zerelio/zerelio/internal/graph"
	"github.com/zerelio/zerelio/internal/storage"
	"github.com/zerelio/zerelio/pkg/types"
)

type Handler struct {
	store  storage.Backend
	jwt    *auth.JWTService
	orch   *discovery.Orchestrator
	graph  *graph.Graph // optional; nil when no PG backend
	logger *slog.Logger
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
	filter := storage.ResourceFilter{
		Type:     r.URL.Query().Get("type"),
		Provider: r.URL.Query().Get("provider"),
		Region:   r.URL.Query().Get("region"),
		Query:    r.URL.Query().Get("q"),
		TenantID: claims.TenantID,
		Limit:    100,
	}

	resources, total, err := h.store.GetResources(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list resources")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"resources": resources,
		"total":     total,
	})
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

func (h *Handler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	scopeID := chi.URLParam(r, "id")
	runs, err := h.store.ListRuns(r.Context(), scopeID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (h *Handler) HandleListPlugins(w http.ResponseWriter, r *http.Request) {
	var plugins []string
	if h.orch != nil {
		plugins = h.orch.ListPlugins()
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": plugins})
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
