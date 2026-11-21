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
	"github.com/zerelio/knowledge/internal/auth"
	"github.com/zerelio/knowledge/internal/discovery"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
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
