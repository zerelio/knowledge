package oidcauth_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/zerelio/zerelio/internal/oidcauth"
)

// statePayload mirrors the unexported struct to build test state tokens.
type statePayload struct {
	Nonce        string    `json:"nonce"`
	CodeVerifier string    `json:"cv"`
	ReturnURL    string    `json:"ru,omitempty"`
	IssuedAt     time.Time `json:"iat"`
}

func TestLoadConfig_MissingIssuer(t *testing.T) {
	t.Setenv("ZERELIO_OIDC_ISSUER", "")
	cfg := oidcauth.LoadConfig()
	if cfg != nil {
		t.Error("expected nil config when ZERELIO_OIDC_ISSUER is unset")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("ZERELIO_OIDC_ISSUER", "https://accounts.google.com")
	t.Setenv("ZERELIO_OIDC_CLIENT_ID", "client_id")
	t.Setenv("ZERELIO_OIDC_CLIENT_SECRET", "client_secret")
	t.Setenv("ZERELIO_OIDC_DEFAULT_ROLES", "")
	t.Setenv("ZERELIO_OIDC_AUTO_PROVISION", "")

	cfg := oidcauth.LoadConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Issuer != "https://accounts.google.com" {
		t.Errorf("Issuer = %q", cfg.Issuer)
	}
	if !cfg.AutoProvision {
		t.Error("AutoProvision should default to true")
	}
	if len(cfg.DefaultRoles) == 0 || cfg.DefaultRoles[0] != "viewer" {
		t.Errorf("DefaultRoles = %v, want [viewer]", cfg.DefaultRoles)
	}
	hasOpenID := false
	for _, s := range cfg.Scopes {
		if s == "openid" {
			hasOpenID = true
		}
	}
	if !hasOpenID {
		t.Errorf("Scopes %v missing openid", cfg.Scopes)
	}
}

func TestLoadConfig_AutoProvisionFalse(t *testing.T) {
	t.Setenv("ZERELIO_OIDC_ISSUER", "https://example.com")
	t.Setenv("ZERELIO_OIDC_AUTO_PROVISION", "false")
	cfg := oidcauth.LoadConfig()
	if cfg.AutoProvision {
		t.Error("expected AutoProvision=false")
	}
}

func TestReturnURL_InvalidState(t *testing.T) {
	url := oidcauth.ReturnURL("not-valid-base64!!!")
	if url != "/" {
		t.Errorf("invalid state should return /, got %q", url)
	}
}

func TestReturnURL_ExpiredState(t *testing.T) {
	payload := statePayload{
		Nonce:     "n",
		IssuedAt:  time.Now().Add(-11 * time.Minute), // expired
		ReturnURL: "/should-not-return",
	}
	raw, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	url := oidcauth.ReturnURL(encoded)
	if url != "/" {
		t.Errorf("expired state should return /, got %q", url)
	}
}

func TestReturnURL_ValidState(t *testing.T) {
	payload := statePayload{
		Nonce:        "testnonce",
		CodeVerifier: "testverifier",
		ReturnURL:    "/console",
		IssuedAt:     time.Now(),
	}
	raw, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(raw)

	got := oidcauth.ReturnURL(encoded)
	if got != "/console" {
		t.Errorf("ReturnURL = %q, want /console", got)
	}
}

func TestReturnURL_EmptyReturn(t *testing.T) {
	payload := statePayload{
		Nonce:    "n",
		IssuedAt: time.Now(),
		// ReturnURL intentionally empty
	}
	raw, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	url := oidcauth.ReturnURL(encoded)
	if url != "/" {
		t.Errorf("empty return_url should give /, got %q", url)
	}
}
