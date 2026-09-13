package authmw

import "strings"

// DegradationDecision is the outcome of CheckDegradation. The middleware
// slice translates it into a response and a log entry; this library only
// computes the decision.
type DegradationDecision int

const (
	// FailOpen means the request may proceed on signature+expiry alone when
	// Redis is unreachable. Only safe (read-only) methods qualify.
	FailOpen DegradationDecision = iota
	// FailClosed means the request must be rejected with 503 when Redis is
	// unreachable. Applies to mutating methods and the auth rotation/revocation
	// endpoints, which depend on the blacklist and epoch for security.
	FailClosed
)

// CheckDegradation returns the degradation decision for a request with the
// given HTTP method and path when Redis is unreachable.
//
// Contract §6 / NFR-02:
//
//   - /auth/refresh and /auth/logout* always fail closed, regardless of method
//     (these endpoints read or write Redis as a security requirement, not as an
//     optimisation).
//   - GET and HEAD otherwise fail open (read-only, no session mutation).
//   - All other methods (POST, PUT, PATCH, DELETE) fail closed.
//
// path must be the raw request path starting with /api/v1 (or whatever the
// mount prefix is). The check uses prefix matching; no router knowledge here.
func CheckDegradation(method, path string) DegradationDecision {
	// Auth mutation endpoints always require Redis for replay/revocation safety.
	if strings.HasPrefix(path, "/api/v1/auth/refresh") ||
		strings.HasPrefix(path, "/api/v1/auth/logout") {
		return FailClosed
	}
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return FailOpen
	default:
		return FailClosed
	}
}
