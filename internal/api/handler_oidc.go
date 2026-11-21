package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zerelio/knowledge/internal/oidcauth"
	"github.com/zerelio/knowledge/pkg/types"
)

// HandleOIDCAuthorize starts the OIDC login flow.
// GET /api/v1/auth/oidc/authorize?return_url=/console
//
// Redirects to the OIDC provider's authorization endpoint.
// If OIDC is not configured, returns 503.
func (h *Handler) HandleOIDCAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		writeError(w, http.StatusServiceUnavailable,
			"OIDC SSO not configured — set ZERELIO_OIDC_ISSUER, ZERELIO_OIDC_CLIENT_ID, "+
				"ZERELIO_OIDC_CLIENT_SECRET, ZERELIO_OIDC_REDIRECT_URL")
		return
	}
	returnURL := r.URL.Query().Get("return_url")
	if returnURL == "" {
		returnURL = "/"
	}

	redirectURL, _, err := h.oidc.AuthorizeURL(returnURL)
	if err != nil {
		h.logger.Error("oidc authorize", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to build authorization URL")
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleOIDCCallback processes the authorization code callback from the provider.
// GET /api/v1/auth/oidc/callback?code=...&state=...
//
// On success, redirects to return_url with tokens in URL fragment:
//
//	{return_url}#access_token={at}&refresh_token={rt}&token_type=Bearer
//
// Tokens in the fragment are never sent to the server on subsequent requests
// (browser-only), which is safer than query parameters.
func (h *Handler) HandleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		writeError(w, http.StatusServiceUnavailable, "OIDC not configured")
		return
	}

	// OAuth2 error from provider.
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		writeError(w, http.StatusBadRequest, fmt.Sprintf("OIDC error: %s — %s", errParam, desc))
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code or state parameter")
		return
	}

	// Exchange code → verify ID token → get claims.
	claims, err := h.oidc.Exchange(r.Context(), code, state)
	if err != nil {
		h.logger.Warn("oidc callback exchange failed", "error", err)
		writeError(w, http.StatusUnauthorized, "OIDC login failed: "+err.Error())
		return
	}

	// Find or provision the user.
	user, err := h.findOrProvisionUser(r, claims)
	if err != nil {
		h.logger.Error("oidc user provisioning", "email", claims.Email, "error", err)
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// Issue Zerelio JWT pair.
	pair, err := h.jwt.IssueTokens(user.ID, user.Email, user.TenantID, user.Roles)
	if err != nil {
		h.logger.Error("oidc issue tokens", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	// Store refresh JTI.
	if err := h.store.StoreRefreshToken(r.Context(), pair.RefreshJTI, user.ID, pair.RefreshTokenExpiry); err != nil {
		h.logger.Error("oidc store refresh token", "error", err)
		// Non-fatal — access token still works.
	}

	// Redirect to return URL with tokens in fragment (never sent to server).
	returnURL := oidcauth.ReturnURL(state)
	fragment := fmt.Sprintf("access_token=%s&refresh_token=%s&token_type=Bearer&expires_in=%d",
		pair.AccessToken, pair.RefreshToken, pair.ExpiresIn)

	h.logger.Info("oidc login successful", "email", user.Email, "user_id", user.ID)
	http.Redirect(w, r, returnURL+"#"+fragment, http.StatusFound)
}

// HandleOIDCStatus returns OIDC configuration status (no secrets).
// GET /api/v1/auth/oidc/status
func (h *Handler) HandleOIDCStatus(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"message": "set ZERELIO_OIDC_ISSUER to enable SSO",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      true,
		"authorize_url": "/api/v1/auth/oidc/authorize",
	})
}

// findOrProvisionUser looks up an existing user by email or creates one
// if ZERELIO_OIDC_AUTO_PROVISION is true (default).
func (h *Handler) findOrProvisionUser(r *http.Request, claims *oidcauth.Claims) (*types.User, error) {
	// Try to find existing user.
	user, err := h.store.GetUserByEmail(r.Context(), claims.Email)
	if err == nil {
		return user, nil
	}

	// Not found — check auto-provision.
	if h.oidcCfg != nil && !h.oidcCfg.AutoProvision {
		return nil, fmt.Errorf("user %s is not registered; ask an admin to create your account", claims.Email)
	}

	// Provision new user.
	roles := []string{"viewer"}
	if h.oidcCfg != nil && len(h.oidcCfg.DefaultRoles) > 0 {
		roles = h.oidcCfg.DefaultRoles
	}

	name := claims.Name
	if name == "" {
		parts := strings.SplitN(claims.Email, "@", 2)
		name = parts[0]
	}
	_ = name // stored implicitly via email; may be used in future display_name column

	newUser := &types.User{
		ID:        fmt.Sprintf("usr_%d", time.Now().UnixNano()),
		Email:     claims.Email,
		Roles:     roles,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	// OIDC users have no local password — set an unusable placeholder hash.
	newUser.PasswordHash = "oidc:" + claims.Sub

	if err := h.store.CreateUser(r.Context(), newUser); err != nil {
		return nil, fmt.Errorf("auto-provision user: %w", err)
	}
	h.logger.Info("oidc: provisioned new user", "email", newUser.Email, "roles", roles)
	return newUser, nil
}
