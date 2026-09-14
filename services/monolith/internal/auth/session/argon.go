package session

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// dummyHash is a valid argon2id hash computed once at program startup against
// a fixed plaintext. It is used when a login attempt specifies an unknown
// email address so that the argon2id verification cost is always paid,
// making the unknown-email timing identical to the bad-password timing
// (FR-08, FR-14, contract §1.1).
//
// It is deliberately not a const: the salt is random so that a precomputed
// rainbow table cannot be cached between processes.
var dummyHash string

func init() {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		// crypto/rand failure is effectively impossible in production, but
		// if it somehow occurs we still need a non-empty hash so the verify
		// path does not short-circuit — use a fixed salt as a last resort.
		salt = []byte("grindstats_dummy")
	}
	// Use the same parameters the credentials slice uses for new passwords
	// (m=64 MiB, t=3 iterations, p=4 lanes) so the timing is equivalent.
	key := argon2.IDKey([]byte("GS::login::timing::dummy::2026"), salt, 3, 64*1024, 4, 32)
	dummyHash = encodeArgon2Hash(3, 64*1024, 4, salt, key)
}

// verifyArgon2ID checks password against encodedHash (PHC string format).
// Returns (true, nil) when the password matches, (false, nil) for a mismatch,
// and (false, err) only when the hash itself is malformed — the caller treats
// a malformed hash the same as a mismatch.
//
// PHC format: $argon2id$v=19$m=M,t=T,p=P$saltB64$hashB64
func verifyArgon2ID(password, encodedHash string) (bool, error) {
	mem, timeCost, threads, salt, hash, err := decodeArgon2Hash(encodedHash)
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(password), salt, timeCost, mem, threads, uint32(len(hash)))
	return subtle.ConstantTimeCompare(candidate, hash) == 1, nil
}

// encodeArgon2Hash serialises argon2id parameters and key material into the
// PHC string format stored by the credentials slice.
func encodeArgon2Hash(timeCost, mem uint32, threads uint8, salt, hash []byte) string {
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		mem, timeCost, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

// decodeArgon2Hash parses a PHC-format argon2id hash string.
func decodeArgon2Hash(encodedHash string) (mem, timeCost uint32, threads uint8, salt, hash []byte, err error) {
	// Expected: $argon2id$v=19$m=M,t=T,p=P$saltB64$hashB64
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		err = fmt.Errorf("session: invalid argon2id hash format (got %d parts)", len(parts))
		return
	}
	// parts[2] = "v=19" — we accept any v value, argon2.IDKey handles it
	// parts[3] = "m=M,t=T,p=P"
	paramStr := parts[3]
	for _, seg := range strings.Split(paramStr, ",") {
		kv := strings.SplitN(seg, "=", 2)
		if len(kv) != 2 {
			err = fmt.Errorf("session: malformed argon2id param segment %q", seg)
			return
		}
		val, parseErr := strconv.ParseUint(kv[1], 10, 32)
		if parseErr != nil {
			err = fmt.Errorf("session: parse argon2id param %q: %w", kv[0], parseErr)
			return
		}
		switch kv[0] {
		case "m":
			mem = uint32(val)
		case "t":
			timeCost = uint32(val)
		case "p":
			threads = uint8(val)
		}
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		err = fmt.Errorf("session: decode argon2id salt: %w", err)
		return
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		err = fmt.Errorf("session: decode argon2id hash: %w", err)
		return
	}
	return
}
