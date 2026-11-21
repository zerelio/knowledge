package types

import "time"

// RefreshToken represents a stored refresh token JTI used for rotation and revocation.
type RefreshToken struct {
	JTI       string    `json:"jti"`
	UserID    string    `json:"user_id"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}
