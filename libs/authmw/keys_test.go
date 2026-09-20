package authmw_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"grindstats/libs/authmw"
)

// genRSAPEM generates a fresh RSA keypair and returns its PEM-encoded
// PKCS#8 private key and PKIX public key.
func genRSAPEM(t *testing.T) (privPEM, pubPEM string, priv *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genRSAPEM: generate key: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("genRSAPEM: marshal private key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("genRSAPEM: marshal public key: %v", err)
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return privPEM, pubPEM, priv
}

func TestLoadKeySetFromPEM_MintAndVerifyRoundTrip(t *testing.T) {
	privPEM, _, _ := genRSAPEM(t)

	ks, err := authmw.LoadKeySetFromPEM(privPEM)
	if err != nil {
		t.Fatalf("LoadKeySetFromPEM: %v", err)
	}

	tokenStr, jti, err := ks.MintAccess("user-1", "sid-1", "user", "free", true)
	if err != nil {
		t.Fatalf("MintAccess: %v", err)
	}
	if tokenStr == "" || jti == "" {
		t.Fatal("MintAccess returned an empty token or jti")
	}

	claims, reason, err := ks.Check(t.Context(), nil, tokenStr, authmw.TypAccess)
	if err != nil {
		t.Fatalf("Check: unexpected error %v", err)
	}
	if reason != "" {
		t.Fatalf("Check: reason = %q, want empty (valid token)", reason)
	}
	if claims.Sub != "user-1" || claims.JTI != jti {
		t.Errorf("Check: claims = %+v, want sub=user-1 jti=%s", claims, jti)
	}
}

func TestLoadKeySetFromPEM_DerivesKIDFromPublicKeyFingerprint(t *testing.T) {
	privPEM, _, _ := genRSAPEM(t)

	ks1, err := authmw.LoadKeySetFromPEM(privPEM)
	if err != nil {
		t.Fatalf("LoadKeySetFromPEM (1st load): %v", err)
	}
	ks2, err := authmw.LoadKeySetFromPEM(privPEM)
	if err != nil {
		t.Fatalf("LoadKeySetFromPEM (2nd load): %v", err)
	}

	if ks1.SigningKID() == "" {
		t.Fatal("SigningKID is empty")
	}
	if ks1.SigningKID() != ks2.SigningKID() {
		t.Errorf("SigningKID is not stable across loads of the same key: %q vs %q",
			ks1.SigningKID(), ks2.SigningKID())
	}
}

func TestLoadKeySetFromPEM_PreviousPublicKeyVerifiesButDoesNotSign(t *testing.T) {
	oldPrivPEM, oldPubPEM, _ := genRSAPEM(t)
	newPrivPEM, _, _ := genRSAPEM(t)

	// Mint a token with the OLD key before rotating.
	oldKS, err := authmw.LoadKeySetFromPEM(oldPrivPEM)
	if err != nil {
		t.Fatalf("load old key: %v", err)
	}
	oldTokenStr, _, err := oldKS.MintAccess("user-1", "sid-1", "user", "free", true)
	if err != nil {
		t.Fatalf("mint with old key: %v", err)
	}

	// Rotate: new key signs, old key's public half is still trusted to verify.
	rotatedKS, err := authmw.LoadKeySetFromPEM(newPrivPEM, oldPubPEM)
	if err != nil {
		t.Fatalf("LoadKeySetFromPEM with previous public key: %v", err)
	}

	// A token minted before rotation must still verify.
	_, reason, err := rotatedKS.Check(t.Context(), nil, oldTokenStr, authmw.TypAccess)
	if err != nil || reason != "" {
		t.Errorf("old token after rotation: reason=%q err=%v, want valid", reason, err)
	}

	// New tokens are signed with the new key's kid, not the old one.
	newTokenStr, _, err := rotatedKS.MintAccess("user-2", "sid-2", "user", "free", true)
	if err != nil {
		t.Fatalf("mint with rotated key: %v", err)
	}
	_, reason, err = rotatedKS.Check(t.Context(), nil, newTokenStr, authmw.TypAccess)
	if err != nil || reason != "" {
		t.Errorf("new token after rotation: reason=%q err=%v, want valid", reason, err)
	}
}

func TestLoadKeySetFromPEM_AcceptsLiteralBackslashN(t *testing.T) {
	privPEM, _, _ := genRSAPEM(t)
	singleLine := strings.ReplaceAll(privPEM, "\n", `\n`)

	ks, err := authmw.LoadKeySetFromPEM(singleLine)
	if err != nil {
		t.Fatalf("LoadKeySetFromPEM with literal backslash-n: %v", err)
	}
	if _, _, err := ks.MintAccess("user-1", "sid-1", "user", "free", true); err != nil {
		t.Errorf("MintAccess after loading single-line PEM: %v", err)
	}
}

func TestLoadKeySetFromPEM_EmptyPrivateKeyIsAnError(t *testing.T) {
	if _, err := authmw.LoadKeySetFromPEM(""); err == nil {
		t.Fatal("LoadKeySetFromPEM(\"\") = nil error, want an error")
	}
}

func TestLoadKeySetFromPEM_MalformedPrivateKeyIsAnError(t *testing.T) {
	if _, err := authmw.LoadKeySetFromPEM("not a pem block"); err == nil {
		t.Fatal("LoadKeySetFromPEM(garbage) = nil error, want an error")
	}
}

func TestLoadKeySetFromPEM_MalformedPreviousPublicKeyIsAnError(t *testing.T) {
	privPEM, _, _ := genRSAPEM(t)
	if _, err := authmw.LoadKeySetFromPEM(privPEM, "not a pem block"); err == nil {
		t.Fatal("LoadKeySetFromPEM with malformed previous public key = nil error, want an error")
	}
}
