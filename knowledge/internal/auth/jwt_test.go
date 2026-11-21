package auth

import (
	"context"
	"testing"
	"time"
)

func TestJWTIssueAndValidate(t *testing.T) {
	_, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	svc := NewJWTService(privKey, 15*time.Minute, 7*24*time.Hour)

	pair, err := svc.IssueTokens("usr_001", "admin@example.com", "tenant_01", []string{"admin"})
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}

	if pair.TokenType != "Bearer" {
		t.Errorf("token type = %q, want Bearer", pair.TokenType)
	}
	if pair.ExpiresIn != 900 {
		t.Errorf("expires_in = %d, want 900", pair.ExpiresIn)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.UserID != "usr_001" {
		t.Errorf("user_id = %q, want usr_001", claims.UserID)
	}
	if claims.Email != "admin@example.com" {
		t.Errorf("email = %q, want admin@example.com", claims.Email)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Errorf("roles = %v, want [admin]", claims.Roles)
	}
	if claims.TenantID != "tenant_01" {
		t.Errorf("tenant_id = %q, want tenant_01", claims.TenantID)
	}
}

func TestJWTRefreshToken(t *testing.T) {
	_, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	svc := NewJWTService(privKey, 15*time.Minute, 7*24*time.Hour)

	pair, err := svc.IssueTokens("usr_001", "user@example.com", "", []string{"viewer"})
	if err != nil {
		t.Fatal(err)
	}

	claims, err := svc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}

	if claims.Subject != "usr_001" {
		t.Errorf("subject = %q, want usr_001", claims.Subject)
	}
	if claims.ID == "" {
		t.Error("refresh token should have a JTI")
	}
}

func TestJWTWrongKey(t *testing.T) {
	_, privKey1, _ := GenerateKeyPair()
	_, privKey2, _ := GenerateKeyPair()

	svc1 := NewJWTService(privKey1, 15*time.Minute, 7*24*time.Hour)
	svc2 := NewJWTService(privKey2, 15*time.Minute, 7*24*time.Hour)

	pair, _ := svc1.IssueTokens("usr_001", "user@example.com", "", []string{"viewer"})

	_, err := svc2.ValidateAccessToken(pair.AccessToken)
	if err == nil {
		t.Error("should reject token signed by different key")
	}
}

func TestClaimsContext(t *testing.T) {
	claims := &Claims{
		UserID: "usr_001",
		Email:  "test@example.com",
		Roles:  []string{"admin"},
	}

	ctx := ContextWithClaims(context.Background(), claims)
	got := ClaimsFromContext(ctx)

	if got == nil {
		t.Fatal("ClaimsFromContext returned nil")
	}
	if got.UserID != "usr_001" {
		t.Errorf("user_id = %q, want usr_001", got.UserID)
	}
}
