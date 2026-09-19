// Package password is the single argon2id implementation for hashing and
// verifying account passwords (SEC-01). Every package that checks a password
// — credentials (register, reset) and session (login) — imports this rather
// than encoding/decoding the PHC format itself, so the format and the cost
// parameters have exactly one owner.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params configures the argon2id cost parameters (SEC-01).
type Params struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
}

// Default are the recommended production parameters from the OWASP Password
// Storage Cheat Sheet (SEC-01).
var Default = Params{
	Time:    3,
	Memory:  64 * 1024, // 64 MB
	Threads: 4,
	KeyLen:  32,
}

// Fast are low-cost parameters for use in tests ONLY. Never use in
// production — they do not provide adequate protection.
var Fast = Params{
	Time:    1,
	Memory:  8 * 1024, // 8 MB
	Threads: 1,
	KeyLen:  32,
}

// Hasher hashes and verifies passwords with argon2id, and pre-computes a
// dummy hash for timing equalization on unknown-account paths (FR-08, §1.1).
type Hasher struct {
	params    Params
	dummyHash string
}

// New constructs a Hasher and pre-computes the dummy hash used on
// unknown-account paths to equalize timing (FR-08). The dummy computation
// runs synchronously; use Fast in tests to keep them fast.
func New(params Params) (*Hasher, error) {
	dummy, err := hashWithParams("gs-dummy-timing-equalization-2026", params)
	if err != nil {
		return nil, fmt.Errorf("password: compute dummy hash: %w", err)
	}
	return &Hasher{params: params, dummyHash: dummy}, nil
}

// Hash returns an argon2id encoded string for the given password. The
// plaintext is never stored, logged, or returned.
func (h *Hasher) Hash(password string) (string, error) {
	return hashWithParams(password, h.params)
}

// Verify returns true if password matches the argon2id encoded string.
// Returns (false, nil) for an empty encoded string (OAuth-only accounts).
func (h *Hasher) Verify(password, encoded string) (bool, error) {
	if encoded == "" {
		return false, nil
	}
	return verify(password, encoded)
}

// DummyVerify runs an argon2id comparison against the pre-computed dummy
// hash and discards the result. Called on unknown-account paths to equalize
// wall-clock time with the known-account path (FR-08, §1.1).
func (h *Hasher) DummyVerify(candidate string) {
	_, _ = verify(candidate, h.dummyHash)
}

// hashWithParams encodes password as "$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>".
func hashWithParams(password string, p Params) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// verify parses the encoded hash and runs a constant-time comparison.
func verify(password, encoded string) (bool, error) {
	// Format: $argon2id$v=19$m=65536,t=3,p=4$<b64salt>$<b64hash>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("password: invalid argon2id format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("password: parse version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("password: unsupported argon2 version %d", version)
	}

	var p Params
	for _, kv := range strings.Split(parts[3], ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return false, fmt.Errorf("password: malformed param %q", kv)
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return false, fmt.Errorf("password: param %s value: %w", k, err)
		}
		switch k {
		case "m":
			p.Memory = uint32(n)
		case "t":
			p.Time = uint32(n)
		case "p":
			p.Threads = uint8(n)
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("password: decode salt: %w", err)
	}
	storedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("password: decode hash: %w", err)
	}
	p.KeyLen = uint32(len(storedHash))

	computed := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return subtle.ConstantTimeCompare(computed, storedHash) == 1, nil
}
