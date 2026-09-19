package credentials

import "grindstats/libs/password"

// Argon2Params, PasswordHasher and friends are aliases onto libs/password —
// the single argon2id implementation shared with the session package. Kept
// under these names so existing callers in this package and its tests are
// unaffected by the move.
type Argon2Params = password.Params

// PasswordHasher is an alias for password.Hasher (SEC-01).
type PasswordHasher = password.Hasher

// DefaultArgon2Params are the recommended production parameters from the
// OWASP Password Storage Cheat Sheet (SEC-01).
var DefaultArgon2Params = password.Default

// FastArgon2Params are low-cost parameters for use in tests ONLY.
// Never use in production — they do not provide adequate protection.
var FastArgon2Params = password.Fast

// NewPasswordHasher constructs a PasswordHasher and pre-computes the dummy
// hash used on unknown-account paths to equalize timing (FR-08). The dummy
// computation runs synchronously; use FastArgon2Params in tests to keep them
// fast.
func NewPasswordHasher(params Argon2Params) (*PasswordHasher, error) {
	return password.New(params)
}
