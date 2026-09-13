package authmw_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/authmw"
)

// newRedisForTest starts a fresh miniredis instance and returns a redis.Client.
// Extracted here so redis_test.go is self-contained (verify_test.go has its own copy).
func newRedisForTest(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

// ─── scenario test ──────────────────────────────────────────────────────────

// TestBlacklistTTL_EqualsTokenRemainingLife asserts that every call to
// BlacklistJTI sets a TTL equal to the token's remaining life — not a
// constant — so blacklist entries expire at the same moment as the tokens
// they cover (NFR-03).
//
// Scenario: AUTH-003 TC-11
// Given:    a jti and an expiry time T seconds in the future
// When:     BlacklistJTI is called
// Then:     the Redis key exists with TTL ≈ T seconds (within 2s tolerance)
//           and IsBlacklisted returns true
func TestBlacklistTTL_EqualsTokenRemainingLife(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newRedisForTest(t)

	// Sub-test each token kind that the blacklist path handles.
	for _, tc := range []struct {
		name      string
		remaining time.Duration
	}{
		{"access_token", 14 * time.Minute},       // ~14 min left
		{"refresh_token", 29 * 24 * time.Hour},   // ~29 days left
		{"nearly_expired", 10 * time.Second},     // 10 seconds left
	} {
		t.Run(tc.name, func(t *testing.T) {
			jti := uuid.New().String()
			exp := time.Now().Add(tc.remaining)

			if err := authmw.BlacklistJTI(ctx, rdb, jti, exp); err != nil {
				t.Fatalf("BlacklistJTI: %v", err)
			}

			// Confirm the key exists.
			isBlacklisted, err := authmw.IsBlacklisted(ctx, rdb, jti)
			if err != nil {
				t.Fatalf("IsBlacklisted: %v", err)
			}
			if !isBlacklisted {
				t.Fatal("want blacklisted=true, got false")
			}

			// Confirm the TTL is within 2 seconds of the remaining life.
			ttl, err := rdb.TTL(ctx, authmw.BlacklistKeyPrefix+jti).Result()
			if err != nil {
				t.Fatalf("TTL: %v", err)
			}
			if ttl <= 0 {
				t.Fatalf("TTL: want positive TTL, got %v", ttl)
			}
			diff := tc.remaining - ttl
			if diff < -2*time.Second || diff > 2*time.Second {
				t.Errorf("TTL mismatch: remaining=%v redis_ttl=%v diff=%v", tc.remaining, ttl, diff)
			}
		})
	}

	// Additional: an already-expired token must NOT produce a blacklist entry.
	t.Run("already_expired", func(t *testing.T) {
		jti := uuid.New().String()
		exp := time.Now().Add(-5 * time.Second) // already expired

		if err := authmw.BlacklistJTI(ctx, rdb, jti, exp); err != nil {
			t.Fatalf("BlacklistJTI: %v", err)
		}

		isBlacklisted, err := authmw.IsBlacklisted(ctx, rdb, jti)
		if err != nil {
			t.Fatalf("IsBlacklisted: %v", err)
		}
		if isBlacklisted {
			t.Error("already-expired token should not produce a blacklist entry")
		}
	})

	// Additional: fast-forward past the TTL and confirm the key vanishes.
	t.Run("expires_after_ttl", func(t *testing.T) {
		jti := uuid.New().String()
		exp := time.Now().Add(5 * time.Second)

		if err := authmw.BlacklistJTI(ctx, rdb, jti, exp); err != nil {
			t.Fatalf("BlacklistJTI: %v", err)
		}

		// Confirm it's there before fast-forward.
		if ok, _ := authmw.IsBlacklisted(ctx, rdb, jti); !ok {
			t.Fatal("want blacklisted before fast-forward")
		}

		mr.FastForward(10 * time.Second) // past the 5s TTL

		if ok, _ := authmw.IsBlacklisted(ctx, rdb, jti); ok {
			t.Error("want NOT blacklisted after TTL expiry")
		}
	})
}

// ─── epoch TTL ──────────────────────────────────────────────────────────────

// TestEpochKey_HasTTL asserts that SetEpoch always writes a key with a TTL.
// A key without a TTL would require a cleanup job, violating NFR-03.
func TestEpochKey_HasTTL(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedisForTest(t)

	userID := "user-epoch-test"
	if err := authmw.SetEpoch(ctx, rdb, userID); err != nil {
		t.Fatalf("SetEpoch: %v", err)
	}

	ttl, err := rdb.TTL(ctx, authmw.EpochKeyPrefix+userID).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("epoch key has no TTL: %v", ttl)
	}
	// 30d ± 5s tolerance.
	want := 30 * 24 * time.Hour
	if ttl < want-5*time.Second || ttl > want+5*time.Second {
		t.Errorf("epoch TTL: want ~%v, got %v", want, ttl)
	}
}

// TestEpochKey_WritesNowPlusOne asserts that epoch = now + 1.
func TestEpochKey_WritesNowPlusOne(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedisForTest(t)

	before := time.Now().Unix()
	if err := authmw.SetEpoch(ctx, rdb, "u2"); err != nil {
		t.Fatalf("SetEpoch: %v", err)
	}
	after := time.Now().Unix()

	epoch, err := authmw.GetEpoch(ctx, rdb, "u2")
	if err != nil {
		t.Fatalf("GetEpoch: %v", err)
	}
	// epoch must be in (before+1 .. after+1] i.e. before+1 <= epoch <= after+1
	if epoch < before+1 || epoch > after+1 {
		t.Errorf("epoch=%d want in [%d, %d]", epoch, before+1, after+1)
	}
}

// TestSessionKey_HasTTL asserts that StoreSession always sets a TTL on the
// hash key (NFR-03).
func TestSessionKey_HasTTL(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedisForTest(t)

	s := authmw.Session{
		SID:         "sid1",
		RefreshJTI:  uuid.New().String(),
		DeviceLabel: "test",
		CreatedAt:   time.Now(),
		LastSeenAt:  time.Now(),
		CSRFToken:   "csrf-abc",
	}
	if err := authmw.StoreSession(ctx, rdb, "user1", s); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}

	ttl, err := rdb.TTL(ctx, authmw.SessionsKeyPrefix+"user1").Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("session hash has no TTL: %v", ttl)
	}
}

// TestLoginBackoff_TTLEqualsDelay asserts that the login_backoff key's TTL
// equals the delay for the stored step, so callers can read Retry-After
// directly from the remaining TTL.
func TestLoginBackoff_TTLEqualsDelay(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedisForTest(t)

	for step := 1; step <= 5; step++ {
		if err := authmw.SetLoginBackoff(ctx, rdb, "user1", step); err != nil {
			t.Fatalf("SetLoginBackoff step %d: %v", step, err)
		}
		expected := authmw.BackoffDelay(step)
		ttl, err := rdb.TTL(ctx, authmw.LoginBackoffPrefix+"user1").Result()
		if err != nil {
			t.Fatalf("TTL step %d: %v", step, err)
		}
		diff := expected - ttl
		if diff < -2*time.Second || diff > 2*time.Second {
			t.Errorf("step %d: TTL=%v want ~%v (diff %v)", step, ttl, expected, diff)
		}
	}
}

// TestLoginFail_SlidingTTL asserts that IncrLoginFailUser resets the 15-minute
// sliding TTL on each increment.
func TestLoginFail_SlidingTTL(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newRedisForTest(t)

	// First failure.
	n, err := authmw.IncrLoginFailUser(ctx, rdb, "user-fail")
	if err != nil || n != 1 {
		t.Fatalf("first incr: n=%d err=%v", n, err)
	}

	// Advance time by 10 minutes (within the 15-minute window).
	mr.FastForward(10 * time.Minute)

	// Second failure — TTL should be reset to 15 minutes.
	n, err = authmw.IncrLoginFailUser(ctx, rdb, "user-fail")
	if err != nil || n != 2 {
		t.Fatalf("second incr: n=%d err=%v", n, err)
	}

	ttl, err := rdb.TTL(ctx, authmw.LoginFailUserPrefix+"user-fail").Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	// TTL should be ~15 minutes, not ~5 minutes.
	if ttl < 14*time.Minute {
		t.Errorf("sliding TTL not reset: got %v, want ~15m", ttl)
	}
}

// TestClearLoginFail_DeletesAllThreeKeys asserts that a successful login clears
// the per-user fail counter, the per-IP fail counter, and the backoff key.
func TestClearLoginFail_DeletesAllThreeKeys(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedisForTest(t)

	authmw.IncrLoginFailUser(ctx, rdb, "u")  //nolint:errcheck
	authmw.IncrLoginFailIP(ctx, rdb, "1.2.3.4")  //nolint:errcheck
	authmw.SetLoginBackoff(ctx, rdb, "u", 1)     //nolint:errcheck

	if err := authmw.ClearLoginFail(ctx, rdb, "u", "1.2.3.4"); err != nil {
		t.Fatalf("ClearLoginFail: %v", err)
	}

	for _, key := range []string{
		authmw.LoginFailUserPrefix + "u",
		authmw.LoginFailIPPrefix + "1.2.3.4",
		authmw.LoginBackoffPrefix + "u",
	} {
		n, _ := rdb.Exists(ctx, key).Result()
		if n != 0 {
			t.Errorf("key %q should be deleted after ClearLoginFail", key)
		}
	}
}
