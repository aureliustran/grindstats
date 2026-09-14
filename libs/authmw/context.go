package authmw

import "context"

// claimsContextKey is an unexported type so that keys from this package never
// collide with keys from other packages, even if they use the same underlying
// value (docs/backend.md §5).
type claimsContextKey struct{}

var claimsKey = claimsContextKey{}

// WithClaims returns a new context with the validated Claims embedded. Called
// by the gateway auth middleware after a successful Check; downstream code
// retrieves the value via ClaimsFromContext without touching Redis (FR-21).
func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFromContext extracts the Claims previously stored by the gateway auth
// middleware. Returns the zero Claims and false when no claims are present
// (e.g. the request did not pass through the auth stage).
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey).(Claims)
	return c, ok
}
