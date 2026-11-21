package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerelio/knowledge/internal/auth"
	"github.com/zerelio/knowledge/internal/discovery"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

type Handler struct {
	store  storage.Backend
	jwt    *auth.JWTService
	orch   *discovery.Orchestrator
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
