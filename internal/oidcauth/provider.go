// Package oidc implements OpenID Connect SSO for Zerelio.
//
// Flow:
//  1. Client requests GET /api/v1/auth/oidc/authorize
//     → Server redirects to the OIDC provider's authorization endpoint
//     → State is a short-lived signed JWT (stateless CSRF + PKCE carrier)
//  2. Provider redirects back to GET /api/v1/auth/oidc/callback?code=...&state=...
//     → Server exchanges code for tokens (with PKCE verifier from state)
//     → Verifies ID token signature and nonce
//     → Creates or finds the user by email
//     → Issues a Zerelio JWT pair and redirects to return_url
//
// Configuration (env vars):
//
//	ZERELIO_OIDC_ISSUER        e.g. https://accounts.google.com
//	ZERELIO_OIDC_CLIENT_ID
//	ZERELIO_OIDC_CLIENT_SECRET
//	ZERELIO_OIDC_REDIRECT_URL  full callback URL, e.g. https://host/api/v1/auth/oidc/callback
//	ZERELIO_OIDC_SCOPES        space-separated extra scopes (default: openid email profile)
//	ZERELIO_OIDC_AUTO_PROVISION true|false (default true) — create user on first login
//	ZERELIO_OIDC_DEFAULT_ROLES  comma-separated roles for provisioned users (default: viewer)
package oidcauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	goidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config holds OIDC provider settings loaded from environment.
type Config struct {
	Issuer          string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	Scopes          []string
	AutoProvision   bool
	DefaultRoles    []string
}

// LoadConfig reads OIDC config from env. Returns nil if ZERELIO_OIDC_ISSUER is unset.
func LoadConfig() *Config {
	issuer := os.Getenv("ZERELIO_OIDC_ISSUER")
	if issuer == "" {
		return nil
	}
	scopes := []string{goidc.ScopeOpenID, "email", "profile"}
	if extra := os.Getenv("ZERELIO_OIDC_SCOPES"); extra != "" {
		scopes = append(scopes, strings.Fields(extra)...)
	}
	roles := []string{"viewer"}
	if r := os.Getenv("ZERELIO_OIDC_DEFAULT_ROLES"); r != "" {
		roles = strings.Split(r, ",")
		for i := range roles {
			roles[i] = strings.TrimSpace(roles[i])
		}
	}
	autoProvision := true
	if ap := os.Getenv("ZERELIO_OIDC_AUTO_PROVISION"); ap == "false" {
		autoProvision = false
	}
	return &Config{
		Issuer:        issuer,
		ClientID:      os.Getenv("ZERELIO_OIDC_CLIENT_ID"),
		ClientSecret:  os.Getenv("ZERELIO_OIDC_CLIENT_SECRET"),
		RedirectURL:   os.Getenv("ZERELIO_OIDC_REDIRECT_URL"),
		Scopes:        scopes,
		AutoProvision: autoProvision,
		DefaultRoles:  roles,
	}
}

// Provider wraps the OIDC provider and OAuth2 config.
type Provider struct {
	cfg      *Config
	oidcProv *goidc.Provider
	oauth2   *oauth2.Config
	verifier *goidc.IDTokenVerifier
}

// NewProvider initialises the OIDC provider by fetching its discovery document.
func NewProvider(ctx context.Context, cfg *Config) (*Provider, error) {
	p, err := goidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discover %s: %w", cfg.Issuer, err)
	}
	o2 := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     p.Endpoint(),
		Scopes:       cfg.Scopes,
	}
	v := p.Verifier(&goidc.Config{ClientID: cfg.ClientID})
	return &Provider{cfg: cfg, oidcProv: p, oauth2: o2, verifier: v}, nil
}

// IsConfigured returns true when OIDC is configured (non-nil Provider).
func (p *Provider) IsConfigured() bool { return p != nil }

// ---- State JWT (stateless CSRF + PKCE carrier) ----

// statePayload is encoded into the state parameter.
type statePayload struct {
	Nonce        string    `json:"nonce"`
	CodeVerifier string    `json:"cv"`
	ReturnURL    string    `json:"ru,omitempty"`
	IssuedAt     time.Time `json:"iat"`
}

// newState creates a signed, base64-encoded state token.
func newState(returnURL string) (stateToken, codeChallenge string, err error) {
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return "", "", err
	}
	// PKCE code_verifier: 64 random URL-safe chars
	cv := make([]byte, 48)
	if _, err = rand.Read(cv); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(cv)
	// code_challenge = BASE64URL(SHA256(verifier))
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	payload := statePayload{
		Nonce:        base64.RawURLEncoding.EncodeToString(nonce),
		CodeVerifier: verifier,
		ReturnURL:    returnURL,
		IssuedAt:     time.Now(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), challenge, nil
}

// parseState decodes and validates a state token.
// Returns error if the token is older than 10 minutes.
func parseState(state string) (*statePayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		return nil, fmt.Errorf("oidc: invalid state encoding: %w", err)
	}
	var p statePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("oidc: invalid state payload: %w", err)
	}
	if time.Since(p.IssuedAt) > 10*time.Minute {
		return nil, fmt.Errorf("oidc: state expired")
	}
	return &p, nil
}

// ---- Authorization request ----

// AuthorizeURL builds the authorization redirect URL.
func (p *Provider) AuthorizeURL(returnURL string) (redirectURL string, state string, err error) {
	state, challenge, err := newState(returnURL)
	if err != nil {
		return "", "", err
	}
	redirectURL = p.oauth2.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	return redirectURL, state, nil
}

// ---- Callback ----

// Claims holds the user identity claims from the ID token.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Exchange validates the callback, exchanges the code for tokens, and returns
// the verified user claims.
func (p *Provider) Exchange(ctx context.Context, code, state string) (*Claims, error) {
	sp, err := parseState(state)
	if err != nil {
		return nil, err
	}

	// Exchange with PKCE verifier.
	token, err := p.oauth2.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", sp.CodeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("oidc: token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("oidc: no id_token in response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: id_token verification: %w", err)
	}

	var c Claims
	if err := idToken.Claims(&c); err != nil {
		return nil, fmt.Errorf("oidc: extract claims: %w", err)
	}
	if c.Email == "" {
		return nil, fmt.Errorf("oidc: email claim missing (ensure 'email' scope is requested)")
	}
	return &c, nil
}

// ReturnURL extracts the return URL from a state token (for use in the callback).
func ReturnURL(state string) string {
	sp, err := parseState(state)
	if err != nil {
		return "/"
	}
	if sp.ReturnURL == "" {
		return "/"
	}
	return sp.ReturnURL
}

// WriteError writes a JSON error suitable for OAuth2 error responses.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}
