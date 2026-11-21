//go:build integration

package storage

import (
	"context"
	"os"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zerelio/knowledge/pkg/types"

	"github.com/testcontainers/testcontainers-go"
)

// skipIfNoDocker skips the test when Docker is unavailable (CI without DinD, local laptops without daemon).
func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("ZERELIO_INTEGRATION") != "1" {
		t.Skip("skipping PGBackend integration tests: set ZERELIO_INTEGRATION=1 to run")
	}
}

func startPostgres(ctx context.Context, t *testing.T, schemaSQL string) string {
	t.Helper()

	ctr, err := tcpostgres.Run(ctx,
		"docker.io/library/postgres:16-alpine",
		tcpostgres.WithDatabase("zerelio_test"),
		tcpostgres.WithUsername("zerelio"),
		tcpostgres.WithPassword("zerelio"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { ctr.Terminate(ctx) }) //nolint:errcheck

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	// Apply schema.
	pgb := NewPGBackend(dsn)
	if err := pgb.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := pgb.pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	pgb.Close() //nolint:errcheck

	return dsn
}

func readSchema(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../db/migrations/001_initial_schema.up.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	return string(data)
}

func newPGBackend(t *testing.T, dsn string) *PGBackend {
	t.Helper()
	ctx := context.Background()
	pgb := NewPGBackend(dsn)
	if err := pgb.Connect(ctx); err != nil {
		t.Fatalf("connect PGBackend: %v", err)
	}
	t.Cleanup(func() { pgb.Close() }) //nolint:errcheck
	return pgb
}

// ---- Tests ----

func TestPGBackend_Users(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	dsn := startPostgres(ctx, t, readSchema(t))
	pg := newPGBackend(t, dsn)

	now := time.Now().UTC().Truncate(time.Millisecond)
	user := &types.User{
		ID:           "usr_001",
		Email:        "alice@example.com",
		PasswordHash: "$argon2id$...",
		Roles:        []string{"admin"},
		TenantID:     "tenant_1",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := pg.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := pg.GetUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ID: got %q want %q", got.ID, user.ID)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "admin" {
		t.Errorf("Roles: got %v want [admin]", got.Roles)
	}

	got2, err := pg.GetUserByID(ctx, "usr_001")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got2.Email != "alice@example.com" {
		t.Errorf("GetUserByID email: got %q", got2.Email)
	}

	count, err := pg.CountUsers(ctx, "tenant_1")
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Errorf("CountUsers: got %d want 1", count)
	}

	if err := pg.DeleteUser(ctx, "usr_001"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := pg.GetUserByEmail(ctx, "alice@example.com"); err == nil {
		t.Error("expected error after delete")
	}
}

func TestPGBackend_ScopesAndRuns(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	dsn := startPostgres(ctx, t, readSchema(t))
	pg := newPGBackend(t, dsn)

	scope := &types.DiscoveryScope{
		ID:         "scope_001",
		TenantID:   "tenant_1",
		Name:       "Production AWS",
		PluginName: "aws",
		Config:     map[string]string{"region": "us-east-1"},
		Status:     "active",
	}

	if err := pg.CreateScope(ctx, scope); err != nil {
		t.Fatalf("CreateScope: %v", err)
	}

	scopes, err := pg.ListScopes(ctx, "tenant_1")
	if err != nil {
		t.Fatalf("ListScopes: %v", err)
	}
	if len(scopes) != 1 {
		t.Fatalf("ListScopes: got %d want 1", len(scopes))
	}
	if scopes[0].Config["region"] != "us-east-1" {
		t.Errorf("Config: got %v", scopes[0].Config)
	}

	// Update scope including last_run.
	now := time.Now().UTC().Truncate(time.Millisecond)
	scope.LastRun = &now
	scope.ResourceCount = 42
	if err := pg.UpdateScope(ctx, scope); err != nil {
		t.Fatalf("UpdateScope: %v", err)
	}

	fetched, err := pg.GetScope(ctx, "scope_001")
	if err != nil {
		t.Fatalf("GetScope: %v", err)
	}
	if fetched.ResourceCount != 42 {
		t.Errorf("ResourceCount: got %d want 42", fetched.ResourceCount)
	}
	if fetched.LastRun == nil {
		t.Error("LastRun should not be nil after update")
	}

	// Store a run.
	run := &types.DiscoveryRun{
		ID:             "run_001",
		ScopeID:        "scope_001",
		Status:         types.StatusCompleted,
		StartedAt:      now,
		ResourcesFound: 42,
	}
	if err := pg.StoreRun(ctx, run); err != nil {
		t.Fatalf("StoreRun: %v", err)
	}

	runs, err := pg.ListRuns(ctx, "scope_001", 10)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListRuns: got %d want 1", len(runs))
	}
	if runs[0].ResourcesFound != 42 {
		t.Errorf("ResourcesFound: got %d", runs[0].ResourcesFound)
	}
}

func TestPGBackend_Resources(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	dsn := startPostgres(ctx, t, readSchema(t))
	pg := newPGBackend(t, dsn)

	now := time.Now().UTC()
	resources := []*types.Resource{
		{
			ID: "res_001", TenantID: "t1", Name: "my-bucket",
			Type: "aws:s3:bucket", Provider: "aws", Region: "us-east-1",
			DiscoveredAt: now, ValidFrom: now,
		},
		{
			ID: "res_002", TenantID: "t1", Name: "my-instance",
			Type: "aws:ec2:instance", Provider: "aws", Region: "us-west-2",
			DiscoveredAt: now, ValidFrom: now,
		},
	}

	if err := pg.StoreResources(ctx, resources); err != nil {
		t.Fatalf("StoreResources: %v", err)
	}

	got, total, err := pg.GetResources(ctx, ResourceFilter{TenantID: "t1"})
	if err != nil {
		t.Fatalf("GetResources: %v", err)
	}
	if total != 2 {
		t.Errorf("total: got %d want 2", total)
	}
	if len(got) != 2 {
		t.Errorf("results: got %d want 2", len(got))
	}

	r, err := pg.GetResourceByID(ctx, "res_001")
	if err != nil {
		t.Fatalf("GetResourceByID: %v", err)
	}
	if r.Name != "my-bucket" {
		t.Errorf("Name: got %q want my-bucket", r.Name)
	}

	// Filter by type.
	buckets, _, err := pg.GetResources(ctx, ResourceFilter{TenantID: "t1", Type: "aws:s3:bucket"})
	if err != nil {
		t.Fatalf("GetResources by type: %v", err)
	}
	if len(buckets) != 1 {
		t.Errorf("filtered: got %d want 1", len(buckets))
	}
}

func TestPGBackend_RefreshTokens(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	dsn := startPostgres(ctx, t, readSchema(t))
	pg := newPGBackend(t, dsn)

	expiry := time.Now().Add(7 * 24 * time.Hour).UTC().Truncate(time.Second)

	if err := pg.StoreRefreshToken(ctx, "jti_abc", "usr_001", expiry); err != nil {
		t.Fatalf("StoreRefreshToken: %v", err)
	}

	rt, err := pg.GetRefreshToken(ctx, "jti_abc")
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if rt.Revoked {
		t.Error("token should not be revoked on creation")
	}
	if rt.UserID != "usr_001" {
		t.Errorf("UserID: got %q want usr_001", rt.UserID)
	}

	if err := pg.RevokeRefreshToken(ctx, "jti_abc"); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}

	rt2, err := pg.GetRefreshToken(ctx, "jti_abc")
	if err != nil {
		t.Fatalf("GetRefreshToken after revoke: %v", err)
	}
	if !rt2.Revoked {
		t.Error("token should be revoked after RevokeRefreshToken")
	}

	// Second token; revoke all by user.
	if err := pg.StoreRefreshToken(ctx, "jti_xyz", "usr_001", expiry); err != nil {
		t.Fatalf("StoreRefreshToken 2: %v", err)
	}
	if err := pg.RevokeUserRefreshTokens(ctx, "usr_001"); err != nil {
		t.Fatalf("RevokeUserRefreshTokens: %v", err)
	}
	rt3, err := pg.GetRefreshToken(ctx, "jti_xyz")
	if err != nil {
		t.Fatalf("GetRefreshToken jti_xyz: %v", err)
	}
	if !rt3.Revoked {
		t.Error("jti_xyz should be revoked after RevokeUserRefreshTokens")
	}
}

func TestPGBackend_Config(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	dsn := startPostgres(ctx, t, readSchema(t))
	pg := newPGBackend(t, dsn)

	if err := pg.SetConfig(ctx, "feature_x", "enabled"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	val, err := pg.GetConfig(ctx, "feature_x")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "enabled" {
		t.Errorf("GetConfig: got %q want enabled", val)
	}

	// Upsert.
	if err := pg.SetConfig(ctx, "feature_x", "disabled"); err != nil {
		t.Fatalf("SetConfig upsert: %v", err)
	}
	val2, _ := pg.GetConfig(ctx, "feature_x")
	if val2 != "disabled" {
		t.Errorf("after upsert: got %q want disabled", val2)
	}
}
