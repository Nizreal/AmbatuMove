package identity

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Tokens struct {
	User                  User      `json:"user"`
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token"`
	TokenType             string    `json:"token_type"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

type Principal struct {
	UserID    string
	Role      string
	SessionID string
}

type PartnerKey struct {
	ID        string    `json:"id"`
	GroupName string    `json:"group_name"`
	Scopes    []string  `json:"scopes"`
	RateTier  string    `json:"rate_tier"`
	CreatedAt time.Time `json:"created_at"`
	RevokedAt *string   `json:"revoked_at,omitempty"`
}
