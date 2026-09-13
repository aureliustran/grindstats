package authmw

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
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

// LoadKeySet reads the private key at privateKeyPath (PEM PKCS#8 or PKCS#1)
// and all *.pub files in pubKeyDir (each named <kid>.pub, PEM PKIX). The
// signing kid is the one whose public half matches the loaded private key.
// It is an error if no public key in pubKeyDir matches the private key —
// misconfigured keys would silently produce unverifiable tokens.
func LoadKeySet(privateKeyPath, pubKeyDir string) (*KeySet, error) {
	privPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("authmw: read private key: %w", err)
	}
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, fmt.Errorf("authmw: no PEM block in %q", privateKeyPath)
	}
	var rsaPriv *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		rsaPriv, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("authmw: parse PKCS1 private key: %w", err)
		}
	default:
		priv, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("authmw: parse private key: %w", err2)
		}
		var ok bool
		rsaPriv, ok = priv.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("authmw: private key is not RSA")
		}
	}

	entries, err := os.ReadDir(pubKeyDir)
	if err != nil {
		return nil, fmt.Errorf("authmw: read public key dir: %w", err)
	}

	pubKeys := make(map[string]*rsa.PublicKey, len(entries))
	signingKID := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pub") {
			continue
		}
		kid := strings.TrimSuffix(e.Name(), ".pub")
		data, err := os.ReadFile(filepath.Join(pubKeyDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("authmw: read public key %s: %w", e.Name(), err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("authmw: no PEM block in %s", e.Name())
		}
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("authmw: parse public key %s: %w", e.Name(), err)
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("authmw: public key %s is not RSA", e.Name())
		}
		pubKeys[kid] = rsaPub
		// Identify which kid corresponds to the signing private key.
		if rsaPriv.PublicKey.N.Cmp(rsaPub.N) == 0 && rsaPriv.PublicKey.E == rsaPub.E {
			signingKID = kid
		}
	}

	if signingKID == "" {
		return nil, fmt.Errorf("authmw: no public key in %q matches the private key", pubKeyDir)
	}
	return &KeySet{
		signingKID: signingKID,
		privateKey: rsaPriv,
		publicKeys: pubKeys,
	}, nil
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
