package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerelio/zerelio/internal/auth"
	"github.com/zerelio/zerelio/internal/discovery"
	"github.com/zerelio/zerelio/internal/storage"
	"github.com/zerelio/zerelio/pkg/types"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	_, privKey, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	jwtSvc := auth.NewJWTService(privKey, 15*time.Minute, 7*24*time.Hour)
	store := storage.NewMemoryBackend()
	store.Connect(context.Background()) //nolint:errcheck
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orch := discovery.NewOrchestrator(store, logger)
	return NewHandler(store, jwtSvc, orch, logger)
}

// injectClaims wraps a handler and injects admin claims (skips real JWT verification).
func injectClaims(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := &auth.Claims{
			UserID:   "usr_test",
			Email:    "admin@test.com",
			Roles:    []string{"admin"},
			TenantID: "tenant_test",
		}
		ctx := auth.ContextWithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestHandleRegisterAndLogin(t *testing.T) {
	h := newTestHandler(t)

	cred := map[string]any{"email": "user@test.com", "password": "hunter2-extra-long-enough", "roles": []string{"viewer"}}
	body, _ := json.Marshal(cred)

	// Register
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleRegister(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Login
	loginBody, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "hunter2-extra-long-enough"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.HandleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var pair auth.TokenPair
	if err := json.NewDecoder(rec.Body).Decode(&pair); err != nil {
		t.Fatalf("decode token pair: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("access_token is empty")
	}
}

func TestHandleRegisterWeakPassword(t *testing.T) {
	h := newTestHandler(t)

	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "short"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleRegister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for weak input, got %d", rec.Code)
	}
}

func TestHandleLoginWrongPassword(t *testing.T) {
	h := newTestHandler(t)

	// Register first
	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "hunter2-extra-long-enough"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	h.HandleRegister(httptest.NewRecorder(), req)

	// Wrong credential
	wrongBody, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "wronginput-extra-long"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBuffer(wrongBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong credential, got %d", rec.Code)
	}
}

func TestHandleCreateAndListResources(t *testing.T) {
	h := newTestHandler(t)

	// Create resources
	payload := map[string]any{
		"resources": []map[string]any{
			{
				"id":       "res_001",
				"name":     "my-bucket",
				"type":     "aws:s3:bucket",
				"provider": "aws",
				"region":   "us-east-1",
			},
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/resources", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	injectClaims(http.HandlerFunc(h.HandleCreateResources)).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create resources status = %d; body: %s", rec.Code, rec.Body.String())
	}

	// List resources
	req = httptest.NewRequest(http.MethodGet, "/api/v1/resources", nil)
	rec = httptest.NewRecorder()
	injectClaims(http.HandlerFunc(h.HandleListResources)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list resources status = %d; body: %s", rec.Code, rec.Body.String())
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result) //nolint:errcheck
	resources := result["resources"].([]any)
	if len(resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(resources))
	}
}

func TestHandleGetResource(t *testing.T) {
	h := newTestHandler(t)

	// Pre-create a resource
	res := &types.Resource{
		ID:           "res_002",
		Name:         "my-instance",
		Type:         "aws:ec2:instance",
		Provider:     "aws",
		Region:       "us-west-2",
		TenantID:     "tenant_test",
		DiscoveredAt: time.Now(),
		ValidFrom:    time.Now(),
	}
	h.store.StoreResources(context.Background(), []*types.Resource{res}) //nolint:errcheck

	// Set up chi router so URLParam works
	r := chi.NewRouter()
	r.Get("/resources/{id}", injectClaims(http.HandlerFunc(h.HandleGetResource)).ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/resources/res_002", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get resource status = %d; body: %s", rec.Code, rec.Body.String())
	}

	var got types.Resource
	json.NewDecoder(rec.Body).Decode(&got) //nolint:errcheck
	if got.ID != "res_002" {
		t.Errorf("resource id = %q, want res_002", got.ID)
	}
}

func TestHandleRefresh(t *testing.T) {
	h := newTestHandler(t)

	// Register + login to get a valid token pair.
	cred := map[string]any{"email": "user@refresh.com", "password": "hunter2-extra-long-enough"}
	body, _ := json.Marshal(cred)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	h.HandleRegister(httptest.NewRecorder(), req)

	loginBody, _ := json.Marshal(map[string]string{"email": "user@refresh.com", "password": "hunter2-extra-long-enough"})
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleLogin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d", rec.Code)
	}

	var pair1 struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
	}
	json.NewDecoder(rec.Body).Decode(&pair1) //nolint:errcheck
	if pair1.RefreshToken == "" {
		t.Fatal("no refresh_token in login response")
	}

	// Use the refresh token.
	refreshBody, _ := json.Marshal(map[string]string{"refresh_token": pair1.RefreshToken})
	req = httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBuffer(refreshBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.HandleRefresh(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d; body: %s", rec.Code, rec.Body.String())
	}

	var pair2 struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(rec.Body).Decode(&pair2) //nolint:errcheck
	if pair2.AccessToken == "" {
		t.Error("no access_token in refresh response")
	}
	if pair2.AccessToken == pair1.AccessToken {
		t.Error("new access token should differ from old one")
	}

	// Using the original refresh token a second time must fail (revoked).
	refreshBody2, _ := json.Marshal(map[string]string{"refresh_token": pair1.RefreshToken})
	req = httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBuffer(refreshBody2))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.HandleRefresh(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("reuse of revoked token: got %d want 401", rec.Code)
	}
}

func TestHandleScopeCRUD(t *testing.T) {
	h := newTestHandler(t)

	// Create a scope.
	payload := map[string]any{
		"name":        "prod-aws",
		"plugin_name": "aws",
		"config":      map[string]string{"region": "us-east-1"},
		"status":      "active",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/discovery/scopes", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	injectClaims(http.HandlerFunc(h.HandleCreateScope)).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scope: got %d; body: %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	json.NewDecoder(rec.Body).Decode(&created) //nolint:errcheck
	scopeID, _ := created["id"].(string)
	if scopeID == "" {
		t.Fatal("no id in create scope response")
	}

	// List scopes.
	req = httptest.NewRequest(http.MethodGet, "/discovery/scopes", nil)
	rec = httptest.NewRecorder()
	injectClaims(http.HandlerFunc(h.HandleListScopes)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list scopes: got %d", rec.Code)
	}
	var listResp map[string]any
	json.NewDecoder(rec.Body).Decode(&listResp) //nolint:errcheck
	scopes, _ := listResp["scopes"].([]any)
	if len(scopes) != 1 {
		t.Errorf("expected 1 scope, got %d", len(scopes))
	}

	// Run scope — no plugin registered, expect 500 "plugin not found".
	r := chi.NewRouter()
	r.Post("/discovery/scopes/{id}/run", injectClaims(http.HandlerFunc(h.HandleRunScope)).ServeHTTP)
	req = httptest.NewRequest(http.MethodPost, "/discovery/scopes/"+scopeID+"/run", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("run scope without plugin: expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleNeighborhoodNoGraph(t *testing.T) {
	h := newTestHandler(t)
	// graph is nil by default — should return 503
	r := chi.NewRouter()
	r.Get("/graph/neighborhood/{id}", injectClaims(http.HandlerFunc(h.HandleNeighborhood)).ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, "/graph/neighborhood/res_001", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 without graph, got %d", rec.Code)
	}
}

func TestHandleSearch(t *testing.T) {
	h := newTestHandler(t)

	// Pre-populate some resources.
	now := time.Now()
	resources := []*types.Resource{
		{ID: "r1", Name: "prod-db", Type: "aws:rds:instance", Provider: "aws", Region: "us-east-1", TenantID: "tenant_test", DiscoveredAt: now, ValidFrom: now},
		{ID: "r2", Name: "staging-bucket", Type: "aws:s3:bucket", Provider: "aws", Region: "eu-west-1", TenantID: "tenant_test", DiscoveredAt: now, ValidFrom: now},
		{ID: "r3", Name: "k8s-cluster", Type: "k8s:cluster", Provider: "k8s", Region: "us-west-2", TenantID: "tenant_test", DiscoveredAt: now, ValidFrom: now},
	}
	h.store.StoreResources(context.Background(), resources) //nolint:errcheck

	// Search with query (searcher is nil, falls back to filter).
	req := httptest.NewRequest(http.MethodGet, "/search?q=prod", nil)
	rec := httptest.NewRecorder()
	injectClaims(http.HandlerFunc(h.HandleSearch)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search status = %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp) //nolint:errcheck
	results, _ := resp["results"].([]any)
	if len(results) == 0 {
		t.Error("expected at least one result for 'prod'")
	}

	// Filter by provider.
	req = httptest.NewRequest(http.MethodGet, "/search?provider=k8s", nil)
	rec = httptest.NewRecorder()
	injectClaims(http.HandlerFunc(h.HandleSearch)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search by provider status = %d", rec.Code)
	}
	var resp2 map[string]any
	json.NewDecoder(rec.Body).Decode(&resp2) //nolint:errcheck
	results2, _ := resp2["results"].([]any)
	if len(results2) != 1 {
		t.Errorf("expected 1 k8s result, got %d", len(results2))
	}
}
