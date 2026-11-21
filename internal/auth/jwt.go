package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID   string   `json:"user_id"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	TenantID string   `json:"tenant_id,omitempty"`
}

type TokenPair struct {
	AccessToken         string    `json:"access_token"`
	RefreshToken        string    `json:"refresh_token"`
	TokenType           string    `json:"token_type"`
	ExpiresIn           int       `json:"expires_in"`
	RefreshJTI          string    `json:"-"` // internal — stored server-side, not exposed to client
	RefreshTokenExpiry  time.Time `json:"-"`
}

type JWTService struct {
	privateKey      ed25519.PrivateKey
	publicKey       ed25519.PublicKey
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
}

func NewJWTService(privateKey ed25519.PrivateKey, accessTTL, refreshTTL time.Duration) *JWTService {
	return &JWTService{
		privateKey:      privateKey,
		publicKey:       privateKey.Public().(ed25519.PublicKey),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		issuer:          "zerelio",
	}
}

func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (s *JWTService) IssueTokens(userID, email, tenantID string, roles []string) (*TokenPair, error) {
	now := time.Now()

	accessClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)),
			ID:        generateTokenID(), // unique JTI per access token
		},
		UserID:   userID,
		Email:    email,
		Roles:    roles,
		TenantID: tenantID,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodEdDSA, accessClaims)
	accessStr, err := accessToken.SignedString(s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshClaims := &jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    s.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTokenTTL)),
		ID:        generateTokenID(),
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodEdDSA, refreshClaims)
	refreshStr, err := refreshToken.SignedString(s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:        accessStr,
		RefreshToken:       refreshStr,
		TokenType:          "Bearer",
		ExpiresIn:          int(s.accessTokenTTL.Seconds()),
		RefreshJTI:         refreshClaims.ID,
		RefreshTokenExpiry: now.Add(s.refreshTokenTTL),
	}, nil
}

func (s *JWTService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

func (s *JWTService) ValidateRefreshToken(tokenStr string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse refresh token: %w", err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}

	return claims, nil
}

func (s *JWTService) PublicKey() ed25519.PublicKey {
	return s.publicKey
}

func generateTokenID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

type contextKey string

const claimsKey contextKey = "claims"

func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}
