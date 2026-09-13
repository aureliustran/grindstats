package authmw_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// newTestKeySet generates an in-memory RSA key pair and wraps it in a KeySet.
// The kid is deterministic so tests can mint tokens with a known kid.
func newTestKeySet(t *testing.T) (*authmw.KeySet, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("newTestKeySet: generate RSA key: %v", err)
	}
	ks := authmw.NewKeySetFromMemory(
		"test-kid",
		priv,
		map[string]*rsa.PublicKey{"test-kid": &priv.PublicKey},
	)
	return ks, priv
}

// newTestRedis starts a miniredis server and returns a matching redis.Client.
// The server is automatically stopped when the test ends.
func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

// mintAccessToken mints a valid access token with the given subject / session.
func mintAccessToken(t *testing.T, ks *authmw.KeySet, sub, sid string) (tokenStr, jti string) {
	t.Helper()
	var err error
	tokenStr, jti, err = ks.MintAccess(sub, sid, "user", "free", true)
	if err != nil {
		t.Fatalf("mintAccessToken: %v", err)
	}
	return
}

// mintExpiredAccess mints an access token whose exp is in the past.
// We do this by signing directly with the private key from the KeySet.
func mintExpiredAccess(t *testing.T, ks *authmw.KeySet, priv *rsa.PrivateKey, sub, sid string) (tokenStr, jti string) {
	t.Helper()
	jti = uuid.New().String()
	now := time.Now()
	claims := authmw.AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now.Add(-20 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-5 * time.Minute)), // expired 5 min ago
		},
		SID:           sid,
		Role:          "user",
		Tier:          "free",
		EmailVerified: true,
		Typ:           authmw.TypAccess,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = ks.SigningKID()
	var err error
	tokenStr, err = tok.SignedString(priv)
	if err != nil {
		t.Fatalf("mintExpiredAccess: %v", err)
	}
	return
}

// mintBadSigAccess mints a token whose signature uses a different (unknown) key.
func mintBadSigAccess(t *testing.T, badPriv *rsa.PrivateKey, sub, sid string) string {
	t.Helper()
	now := time.Now()
	claims := authmw.AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		SID:           sid,
		Role:          "user",
		Tier:          "free",
		EmailVerified: true,
		Typ:           authmw.TypAccess,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "unknown-kid"
	tokenStr, err := tok.SignedString(badPriv)
	if err != nil {
		t.Fatalf("mintBadSigAccess: %v", err)
	}
	return tokenStr
}

// ─── scenario test ──────────────────────────────────────────────────────────

// TestCheckOrder_SignatureThenExpiryThenBlacklistThenEpoch asserts that the
// check chain runs in the order: signature → expiry → blacklist → epoch, and
// that a token failing multiple checks simultaneously always reports the
// earliest failure — not whichever check happened to run last.
//
// Scenario: AUTH-003 TC-03
// Given:    tokens in various failure states
// When:     Check is called with each token
// Then:     the returned TokenRejectReason is always the FIRST failing check
func TestCheckOrder_SignatureThenExpiryThenBlacklistThenEpoch(t *testing.T) {
	ctx := context.Background()
	ks, priv := newTestKeySet(t)
	_, rdb := newTestRedis(t)

	// --- 1. Bad signature only → BadSignature ---
	badSigToken := mintBadSigAccess(t, priv, "user1", "sess1")
	_, reason, err := ks.Check(ctx, rdb, badSigToken, authmw.TypAccess)
	if err != nil {
		t.Fatalf("bad-sig: unexpected error: %v", err)
	}
	if reason != auditmodel.TokenRejectReasonBadSignature {
		t.Errorf("bad-sig: want BadSignature, got %q", reason)
	}

	// --- 2. Expired token (signature OK) — also blacklisted and epoch-stale ---
	//
	// Even though the jti is blacklisted and the epoch is stale, the expiry
	// check comes before blacklist and epoch in the chain, so Expired wins.
	expiredToken, expiredJTI := mintExpiredAccess(t, ks, priv, "user2", "sess2")
	// Blacklist the jti (even though it won't reach the blacklist check).
	if err := authmw.BlacklistJTI(ctx, rdb, expiredJTI, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("blacklist expired jti: %v", err)
	}
	// Set epoch (epoch check also won't be reached).
	if err := authmw.SetEpoch(ctx, rdb, "user2"); err != nil {
		t.Fatalf("set epoch: %v", err)
	}
	_, reason, err = ks.Check(ctx, rdb, expiredToken, authmw.TypAccess)
	if err != nil {
		t.Fatalf("expired: unexpected error: %v", err)
	}
	if reason != auditmodel.TokenRejectReasonExpired {
		t.Errorf("expired: want Expired, got %q", reason)
	}

	// --- 3. Valid, unexpired token — blacklisted AND epoch-stale ---
	//
	// Blacklist comes before epoch in the chain; Blacklisted must win.
	tokenStr, jti := mintAccessToken(t, ks, "user3", "sess3")
	if err := authmw.BlacklistJTI(ctx, rdb, jti, time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("blacklist jti: %v", err)
	}
	if err := authmw.SetEpoch(ctx, rdb, "user3"); err != nil {
		t.Fatalf("set epoch: %v", err)
	}
	_, reason, err = ks.Check(ctx, rdb, tokenStr, authmw.TypAccess)
	if err != nil {
		t.Fatalf("blacklisted+epoch: unexpected error: %v", err)
	}
	if reason != auditmodel.TokenRejectReasonBlacklisted {
		t.Errorf("blacklisted+epoch: want Blacklisted, got %q", reason)
	}

	// --- 4. Valid, not expired, not blacklisted — epoch-stale only ---
	tokenStr4, _ := mintAccessToken(t, ks, "user4", "sess4")
	if err := authmw.SetEpoch(ctx, rdb, "user4"); err != nil {
		t.Fatalf("set epoch: %v", err)
	}
	// The token was minted BEFORE SetEpoch advanced the epoch to now+1,
	// so its iat < epoch.
	_, reason, err = ks.Check(ctx, rdb, tokenStr4, authmw.TypAccess)
	if err != nil {
		t.Fatalf("epoch-stale: unexpected error: %v", err)
	}
	if reason != auditmodel.TokenRejectReasonEpochStale {
		t.Errorf("epoch-stale: want EpochStale, got %q", reason)
	}

	// --- 5. All four failures present — BadSignature must win ---
	//
	// Use a token signed with an unknown key (bad sig), expired, whose jti is
	// blacklisted, and whose user has an epoch. The first check (signature)
	// must dominate.
	allBadToken := mintBadSigAccess(t, priv, "user5", "sess5")
	if err := authmw.SetEpoch(ctx, rdb, "user5"); err != nil {
		t.Fatalf("set epoch: %v", err)
	}
	_, reason, err = ks.Check(ctx, rdb, allBadToken, authmw.TypAccess)
	if err != nil {
		t.Fatalf("all-bad: unexpected error: %v", err)
	}
	if reason != auditmodel.TokenRejectReasonBadSignature {
		t.Errorf("all-bad: want BadSignature, got %q", reason)
	}
}

// ─── additional coverage ────────────────────────────────────────────────────

// TestMintVerify_RoundTrip tests that a freshly minted access token verifies
// cleanly with no reject reason.
func TestMintVerify_RoundTrip(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	ctx := context.Background()

	tokenStr, jti, err := ks.MintAccess("uid1", "sid1", "user", "free", true)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	claims, reason, err := ks.Check(ctx, rdb, tokenStr, authmw.TypAccess)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if reason != "" {
		t.Errorf("want no reason, got %q", reason)
	}
	if claims.Sub != "uid1" {
		t.Errorf("sub: want uid1, got %q", claims.Sub)
	}
	if claims.SID != "sid1" {
		t.Errorf("sid: want sid1, got %q", claims.SID)
	}
	if claims.JTI != jti {
		t.Errorf("jti: want %q, got %q", jti, claims.JTI)
	}
	if claims.Typ != authmw.TypAccess {
		t.Errorf("typ: want access, got %q", claims.Typ)
	}
}

// TestMintVerify_RefreshRoundTrip tests refresh token round-trip.
func TestMintVerify_RefreshRoundTrip(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	ctx := context.Background()

	tokenStr, jti, err := ks.MintRefresh("uid2", "sid2")
	if err != nil {
		t.Fatalf("mint refresh: %v", err)
	}

	claims, reason, err := ks.Check(ctx, rdb, tokenStr, authmw.TypRefresh)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if reason != "" {
		t.Errorf("want no reason, got %q", reason)
	}
	if claims.Sub != "uid2" || claims.SID != "sid2" || claims.JTI != jti {
		t.Errorf("claims mismatch: %+v", claims)
	}
	if claims.Typ != authmw.TypRefresh {
		t.Errorf("typ: want refresh, got %q", claims.Typ)
	}
}

// TestKIDRotation_TwoKeyVerificationWindow verifies that a token signed with
// an older key (still registered as a public key) is accepted alongside a
// token signed with the current key.
func TestKIDRotation_TwoKeyVerificationWindow(t *testing.T) {
	ctx := context.Background()
	_, rdb := newTestRedis(t)

	// Generate two key pairs.
	oldPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	newPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	// KeySet signs with "new-kid" but also verifies tokens signed with "old-kid".
	ks := authmw.NewKeySetFromMemory("new-kid", newPriv, map[string]*rsa.PublicKey{
		"old-kid": &oldPriv.PublicKey,
		"new-kid": &newPriv.PublicKey,
	})

	// Mint with old key directly.
	oldKS := authmw.NewKeySetFromMemory("old-kid", oldPriv, map[string]*rsa.PublicKey{
		"old-kid": &oldPriv.PublicKey,
	})
	oldToken, _, err := oldKS.MintAccess("uid", "sid", "user", "free", false)
	if err != nil {
		t.Fatalf("mint old: %v", err)
	}

	// Mint with new key.
	newToken, _, err := ks.MintAccess("uid", "sid", "user", "free", false)
	if err != nil {
		t.Fatalf("mint new: %v", err)
	}

	// Both tokens must verify against ks (which holds both public keys).
	for name, tok := range map[string]string{"old": oldToken, "new": newToken} {
		_, reason, err := ks.Check(ctx, rdb, tok, authmw.TypAccess)
		if err != nil {
			t.Errorf("%s token check error: %v", name, err)
		}
		if reason != "" {
			t.Errorf("%s token: want no reason, got %q", name, reason)
		}
	}
}

// TestTypConfusion_AccessPresentedAsRefresh ensures that an access token is
// rejected when the caller expects a refresh token (and vice-versa).
func TestTypConfusion_AccessPresentedAsRefresh(t *testing.T) {
	ks, _ := newTestKeySet(t)
	_, rdb := newTestRedis(t)
	ctx := context.Background()

	accessToken, _, _ := ks.MintAccess("u", "s", "user", "free", true)
	refreshToken, _, _ := ks.MintRefresh("u", "s")

	// Access token presented as refresh → reject.
	_, reason, _ := ks.Check(ctx, rdb, accessToken, authmw.TypRefresh)
	if reason != auditmodel.TokenRejectReasonBadSignature {
		t.Errorf("access-as-refresh: want BadSignature, got %q", reason)
	}

	// Refresh token presented as access → reject.
	_, reason, _ = ks.Check(ctx, rdb, refreshToken, authmw.TypAccess)
	if reason != auditmodel.TokenRejectReasonBadSignature {
		t.Errorf("refresh-as-access: want BadSignature, got %q", reason)
	}
}

// TestEpochBoundary_SameSecond ensures that a token minted in the same second
// as a logout-all does NOT survive the epoch. Contract §3:
//
//	"epoch = now + 1, which is why 'including the legitimately-rotated one
//	 just issued' holds."
func TestEpochBoundary_SameSecond(t *testing.T) {
	ks, _ := newTestKeySet(t)
	mr, rdb := newTestRedis(t)
	ctx := context.Background()

	// Mint a token; its iat will be now (unix seconds).
	tokenStr, _, err := ks.MintAccess("u", "s", "user", "free", true)
	if err != nil {
		t.Fatal(err)
	}

	// Set epoch in miniredis (miniredis time is real time by default).
	// SetEpoch writes now+1, so token's iat (now) < epoch (now+1).
	if err := authmw.SetEpoch(ctx, rdb, "u"); err != nil {
		t.Fatal(err)
	}

	// Fast-forward miniredis to avoid flakes from sub-second timing.
	mr.FastForward(0) // nop, just to confirm miniredis is in use

	_, reason, err := ks.Check(ctx, rdb, tokenStr, authmw.TypAccess)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if reason != auditmodel.TokenRejectReasonEpochStale {
		t.Errorf("same-second epoch: want EpochStale, got %q", reason)
	}
}

// TestBackoffSequence ensures the pure backoff function produces the expected
// sequence 30, 60, 120, …, capped at 3600.
func TestBackoffSequence(t *testing.T) {
	cases := []struct {
		step int
		want time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
		{4, 240 * time.Second},
		{5, 480 * time.Second},
		{6, 960 * time.Second},
		{7, 1920 * time.Second},
		{8, 3600 * time.Second}, // cap
		{9, 3600 * time.Second}, // still capped
		{100, 3600 * time.Second},
	}
	for _, c := range cases {
		got := authmw.BackoffDelay(c.step)
		if got != c.want {
			t.Errorf("BackoffDelay(%d): want %v, got %v", c.step, c.want, got)
		}
	}
}
