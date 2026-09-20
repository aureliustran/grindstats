package authmw

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token TTLs (contract §2.1).
const (
	AccessTTL  = 15 * time.Minute
	RefreshTTL = 30 * 24 * time.Hour
)

// KeySet holds one RS256 signing key and a set of verifying public keys.
// One key signs; many can verify — this is the two-key rotation window
// described in contract SEC-02: when you rotate, keep the old public key
// registered so tokens signed with it remain verifiable until they expire.
type KeySet struct {
	// signingKID is the kid used when minting new tokens.
	signingKID string
	// privateKey is the RS256 signing key.
	privateKey *rsa.PrivateKey
	// publicKeys maps kid → RSA public key.
	publicKeys map[string]*rsa.PublicKey
}

// LoadKeySetFromPEM builds a KeySet from PEM-encoded key material passed as
// plain strings — e.g. the value of an environment variable — rather than
// file paths. This is deliberately the only way to load a KeySet outside of
// tests: it matches how the key actually arrives in every real environment
// (SSM Parameter Store SecureString injected as an env var in prod —
// docs/deployment-aws.md §4 — or a value pasted into .env locally), so there
// is one code path instead of a files-in-dev / env-vars-in-prod split.
//
// privatePEM is the RS256 signing key (PKCS#1 or PKCS#8). Its public half is
// derived automatically — nothing separate needs to be generated or pasted
// for the common case of "one active key, no rotation in progress".
//
// previousPublicPEMs are additional PEM-encoded RSA public keys accepted for
// verification only, not signing (SEC-02's two-key rotation window): when
// rotating to a new private key, pass the outgoing key's public half here so
// tokens it already signed keep verifying until they expire. Omit it when
// there is no rotation underway, which is the normal case.
//
// The kid for every key (signing and previous) is derived from the first 16
// hex characters of the SHA-256 digest of its DER-encoded SubjectPublicKeyInfo
// — stable, collision-resistant in practice, and nothing the caller has to
// track or paste alongside the key material.
//
// privatePEM may use literal "\n" sequences instead of real newlines, which
// is how a multi-line PEM block survives being pasted into a single-line
// .env value; both forms are accepted.
func LoadKeySetFromPEM(privatePEM string, previousPublicPEMs ...string) (*KeySet, error) {
	if strings.TrimSpace(privatePEM) == "" {
		return nil, fmt.Errorf("authmw: private key PEM is empty")
	}
	rsaPriv, err := parsePrivateKeyPEM(privatePEM)
	if err != nil {
		return nil, fmt.Errorf("authmw: private key: %w", err)
	}

	signingKID := keyID(&rsaPriv.PublicKey)
	pubKeys := map[string]*rsa.PublicKey{signingKID: &rsaPriv.PublicKey}

	for i, prevPEM := range previousPublicPEMs {
		if strings.TrimSpace(prevPEM) == "" {
			continue
		}
		rsaPub, err := parsePublicKeyPEM(prevPEM)
		if err != nil {
			return nil, fmt.Errorf("authmw: previous public key #%d: %w", i, err)
		}
		pubKeys[keyID(rsaPub)] = rsaPub
	}

	return &KeySet{
		signingKID: signingKID,
		privateKey: rsaPriv,
		publicKeys: pubKeys,
	}, nil
}

// keyID derives a stable kid from a public key's fingerprint, so nothing
// needs to name or track kids by hand (docs/audit-and-errors.md's compact-code
// pattern uses the same "derive an identifier, don't ask for one" approach).
func keyID(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		// MarshalPKIXPublicKey only fails for key types it doesn't support;
		// an *rsa.PublicKey is always supported, so this is unreachable.
		panic(fmt.Sprintf("authmw: marshal public key: %v", err))
	}
	sum := sha256.Sum256(der)
	return "key-" + hex.EncodeToString(sum[:])[:16]
}

// parsePrivateKeyPEM decodes an RSA private key from a PEM string (PKCS#1 or
// PKCS#8), accepting literal "\n" in place of real newlines.
func parsePrivateKeyPEM(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(normalizePEM(pemStr)))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8: %w", err)
	}
	rsaPriv, ok := priv.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}
	return rsaPriv, nil
}

// parsePublicKeyPEM decodes an RSA public key from a PEM string (PKIX).
func parsePublicKeyPEM(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(normalizePEM(pemStr)))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKIX: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}
	return rsaPub, nil
}

// normalizePEM turns literal backslash-n sequences into real newlines, so a
// PEM block pasted as a single-line .env value parses the same as one kept
// as a real multi-line string.
func normalizePEM(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), `\n`, "\n")
}

// NewKeySetFromMemory constructs a KeySet from in-memory key material. This
// is primarily for unit tests that want to avoid key files on disk.
func NewKeySetFromMemory(signingKID string, priv *rsa.PrivateKey, pubKeys map[string]*rsa.PublicKey) *KeySet {
	return &KeySet{
		signingKID: signingKID,
		privateKey: priv,
		publicKeys: pubKeys,
	}
}

// SigningKID returns the kid that new tokens are minted with.
func (ks *KeySet) SigningKID() string { return ks.signingKID }

// MintAccess mints a new RS256 access token.
// Returns (tokenString, jti, error). The jti is the unit of blacklisting;
// keep it distinct from the sid (stable session identifier).
func (ks *KeySet) MintAccess(sub, sid, role, tier string, emailVerified bool) (tokenStr, jti string, err error) {
	now := time.Now()
	jti = uuid.New().String()
	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)),
		},
		SID:           sid,
		Role:          role,
		Tier:          tier,
		EmailVerified: emailVerified,
		Typ:           TypAccess,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = ks.signingKID
	tokenStr, err = tok.SignedString(ks.privateKey)
	return
}

// MintRefresh mints a new RS256 refresh token.
// Returns (tokenString, jti, error). A fresh jti is generated on every call;
// on rotation the old jti is blacklisted and the new one stored.
func (ks *KeySet) MintRefresh(sub, sid string) (tokenStr, jti string, err error) {
	now := time.Now()
	jti = uuid.New().String()
	claims := RefreshClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTTL)),
		},
		SID: sid,
		Typ: TypRefresh,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = ks.signingKID
	tokenStr, err = tok.SignedString(ks.privateKey)
	return
}

// keyFunc is the jwt.Keyfunc for ParseWithClaims. It supports the two-key
// rotation window (SEC-02) by selecting the public key by kid header.
func (ks *KeySet) keyFunc(tok *jwt.Token) (interface{}, error) {
	if tok.Method.Alg() != jwt.SigningMethodRS256.Alg() {
		return nil, fmt.Errorf("authmw: unexpected signing method %q", tok.Method.Alg())
	}
	kid, _ := tok.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("authmw: token missing kid header")
	}
	pub, ok := ks.publicKeys[kid]
	if !ok {
		return nil, fmt.Errorf("authmw: unknown kid %q", kid)
	}
	return pub, nil
}

// extractClaims unpacks a MapClaims into the flat Claims struct the rest of
// the library operates on.
func extractClaims(mc jwt.MapClaims) (Claims, error) {
	sub, _ := mc.GetSubject()
	jti, _ := mc["jti"].(string)
	sid, _ := mc["sid"].(string)
	typ, _ := mc["typ"].(string)
	role, _ := mc["role"].(string)
	tier, _ := mc["tier"].(string)
	emailVerified, _ := mc["email_verified"].(bool)

	var iat int64
	if issuedAt, err := mc.GetIssuedAt(); err == nil && issuedAt != nil {
		iat = issuedAt.Unix()
	}

	return Claims{
		Sub:           sub,
		SID:           sid,
		JTI:           jti,
		IAT:           iat,
		Typ:           typ,
		Role:          role,
		Tier:          tier,
		EmailVerified: emailVerified,
	}, nil
}
