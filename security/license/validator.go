// Package license validates Zerelio commercial license tokens.
//
// Licenses are signed Ed25519 JWTs issued by the Zerelio license server.
// The public key is distributed with the product; only Zerelio holds the
// private signing key.
//
// License token claims:
//
//	{
//	  "sub":          "cust_abc123",         // customer ID
//	  "tenant_id":    "ten_abc",             // primary tenant
//	  "features":     ["rbac","threat"],     // enabled feature flags
//	  "max_users":    50,                    // 0 = unlimited
//	  "iss":          "license.zerelio.io",
//	  "exp":          1735689600,            // Unix seconds
//	  "iat":          1704153600
//	}
package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const licenseIssuer = "license.zerelio.io"

// License describes the entitlements granted to a customer.
type License struct {
	CustomerID string    `json:"customer_id"`
	TenantID   string    `json:"tenant_id"`
	Features   []string  `json:"features"`
	MaxUsers   int       `json:"max_users"` // 0 = unlimited
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// claims is the JWT payload structure for a license token.
type claims struct {
	jwt.RegisteredClaims
	TenantID string   `json:"tenant_id"`
	Features []string `json:"features"`
	MaxUsers int      `json:"max_users"`
}

// Validator validates license JWTs using the Zerelio license server's public key.
type Validator struct {
	publicKey ed25519.PublicKey
}

// NewValidator parses a PEM-encoded Ed25519 public key and returns a Validator.
func NewValidator(publicKeyPEM string) (*Validator, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("license: invalid PEM block")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("license: parse public key: %w", err)
	}

	ed, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("license: expected Ed25519 key, got %T", key)
	}

	return &Validator{publicKey: ed}, nil
}

// NewValidatorFromKey creates a Validator directly from a raw Ed25519 public key.
// Useful in tests where you generate the key pair in-process.
func NewValidatorFromKey(pub ed25519.PublicKey) *Validator {
	return &Validator{publicKey: pub}
}

// Validate parses and verifies a license token. Returns the license if valid.
func (v *Validator) Validate(tokenStr string) (*License, error) {
	var c claims
	token, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("license: unexpected signing method %v", t.Header["alg"])
		}
		return v.publicKey, nil
	},
		jwt.WithIssuer(licenseIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("license: invalid token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("license: token not valid")
	}

	iat, _ := c.GetIssuedAt()
	exp, _ := c.GetExpirationTime()

	return &License{
		CustomerID: c.Subject,
		TenantID:   c.TenantID,
		Features:   c.Features,
		MaxUsers:   c.MaxUsers,
		IssuedAt:   iat.Time,
		ExpiresAt:  exp.Time,
	}, nil
}

// IsFeatureEnabled returns true if the license grants the named feature.
func (v *Validator) IsFeatureEnabled(lic *License, feature string) bool {
	for _, f := range lic.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// IsExpired returns true if the license has passed its expiry.
func (v *Validator) IsExpired(lic *License) bool {
	return time.Now().After(lic.ExpiresAt)
}

// UserAllowed returns true if the given user count does not exceed MaxUsers.
// A MaxUsers of 0 means unlimited.
func (v *Validator) UserAllowed(lic *License, currentUsers int) bool {
	if lic.MaxUsers == 0 {
		return true
	}
	return currentUsers <= lic.MaxUsers
}
