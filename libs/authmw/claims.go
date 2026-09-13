// Package authmw is the single place where JWT token semantics and Redis
// auth-state are implemented. Every auth decision in the system flows through
// this library; no handler or domain package reimplements what is here.
//
// It has no knowledge of Gin, HTTP handlers, or any services/ package.
// The gateway middleware, the session handler and the credentials handler all
// take this library as a dependency — none of them implement a line of it.
package authmw

import "github.com/golang-jwt/jwt/v5"

// Token type constants (contract §2.1, "typ" claim).
const (
	TypAccess  = "access"
	TypRefresh = "refresh"
)

// AccessClaims is the full payload of a 15-minute access token.
// Contract §2.1: sub, sid, role, tier, email_verified, jti, iat, exp, typ:"access"
//
// sub → RegisteredClaims.Subject
// jti → RegisteredClaims.ID  (unit of blacklisting; new on every mint)
// iat → RegisteredClaims.IssuedAt
// exp → RegisteredClaims.ExpiresAt
type AccessClaims struct {
	jwt.RegisteredClaims
	// SID is the session id — stable across every rotation within one login.
	// Confusing SID with JTI (RegisteredClaims.ID) produces a system where
	// rotation kills the session it just refreshed.
	SID           string `json:"sid"`
	Role          string `json:"role"`
	Tier          string `json:"tier"`
	EmailVerified bool   `json:"email_verified"`
	Typ           string `json:"typ"`
}

// RefreshClaims is the payload of a 30-day refresh token.
// Contract §2.1: sub, sid, jti, iat, exp, typ:"refresh"
type RefreshClaims struct {
	jwt.RegisteredClaims
	SID string `json:"sid"`
	Typ string `json:"typ"`
}

// Claims is the decoded, check-chain-passed claim set returned by Check.
// Fields present only in access tokens (Role, Tier, EmailVerified) are
// zero-valued on a refresh token; callers check Typ first.
type Claims struct {
	// Sub is the user_id (RegisteredClaims.Subject).
	Sub string
	// SID is the session id — stable across rotations within one login.
	SID string
	// JTI is the token id — new on every mint, unit of blacklisting.
	JTI string
	// IAT is the issued-at unix timestamp (second resolution).
	IAT int64
	// Typ is "access" or "refresh".
	Typ string
	// Access-only fields (zero for refresh tokens).
	Role          string
	Tier          string
	EmailVerified bool
}
