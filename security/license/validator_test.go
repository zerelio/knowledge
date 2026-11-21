package license_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zerelio/zerelio/security/license"
)

// issueTestToken signs a license token with a known private key.
func issueTestToken(t *testing.T, priv ed25519.PrivateKey, features []string, ttl time.Duration) string {
	t.Helper()
	now := time.Now()
	c := struct {
		jwt.RegisteredClaims
		TenantID string   `json:"tenant_id"`
		Features []string `json:"features"`
		MaxUsers int      `json:"max_users"`
	}{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "cust_test",
			Issuer:    "license.zerelio.io",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		TenantID: "ten_test",
		Features: features,
		MaxUsers: 10,
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c).SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func newKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestValidate_Valid(t *testing.T) {
	pub, priv := newKeyPair(t)
	v := license.NewValidatorFromKey(pub)

	tok := issueTestToken(t, priv, []string{"rbac", "threat"}, time.Hour)
	lic, err := v.Validate(tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lic.CustomerID != "cust_test" {
		t.Errorf("CustomerID = %q, want cust_test", lic.CustomerID)
	}
	if !v.IsFeatureEnabled(lic, "rbac") {
		t.Error("rbac feature should be enabled")
	}
	if v.IsFeatureEnabled(lic, "unknown") {
		t.Error("unknown feature should not be enabled")
	}
}

func TestValidate_Expired(t *testing.T) {
	pub, priv := newKeyPair(t)
	v := license.NewValidatorFromKey(pub)

	tok := issueTestToken(t, priv, nil, -time.Minute) // already expired
	_, err := v.Validate(tok)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidate_WrongKey(t *testing.T) {
	_, priv := newKeyPair(t)
	pub2, _ := newKeyPair(t) // different key
	v := license.NewValidatorFromKey(pub2)

	tok := issueTestToken(t, priv, nil, time.Hour)
	_, err := v.Validate(tok)
	if err == nil {
		t.Error("expected error for wrong key")
	}
}

func TestNewValidatorFromPEM(t *testing.T) {
	pub, _ := newKeyPair(t)

	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	v, err := license.NewValidator(pemStr)
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	if v == nil {
		t.Error("validator is nil")
	}
}

func TestUserAllowed(t *testing.T) {
	pub, priv := newKeyPair(t)
	v := license.NewValidatorFromKey(pub)
	tok := issueTestToken(t, priv, nil, time.Hour) // MaxUsers = 10
	lic, _ := v.Validate(tok)

	if !v.UserAllowed(lic, 10) {
		t.Error("10 users should be allowed with MaxUsers=10")
	}
	if v.UserAllowed(lic, 11) {
		t.Error("11 users should not be allowed with MaxUsers=10")
	}
}
