package authmw

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis key prefixes (contract §3).
const (
	BlacklistKeyPrefix    = "blacklist:"
	EpochKeyPrefix        = "user_blacklist_epoch:"
	SessionsKeyPrefix     = "user_sessions:"
	LoginFailUserPrefix   = "login_fail:"
	LoginFailIPPrefix     = "login_fail_ip:"
	LoginBackoffPrefix    = "login_backoff:"
	RateLimitKeyPrefix    = "ratelimit:"
)

// TTLs for long-lived keys.
const (
	epochTTL   = 30 * 24 * time.Hour
	sessionTTL = 30 * 24 * time.Hour
	// loginFailTTL is a sliding 15-minute window (reset on each failure).
	loginFailTTL = 15 * time.Minute
)

// ============================================================
// Blacklist — blacklist:{jti}
// ============================================================

// BlacklistJTI blacklists a token by its jti with a TTL equal to the token's
// remaining life (time until exp). Contract §3:
//
//	"TTL is the token's remaining life computed from exp — never a constant.
//	 A blacklist entry outliving its token is a leak; one expiring early is
//	 a revocation that silently stops working (NFR-03)."
//
// If the token is already expired (remaining <= 0) the key is not written;
// the expiry check in Check() handles it.
func BlacklistJTI(ctx context.Context, rdb *redis.Client, jti string, exp time.Time) error {
	remaining := time.Until(exp)
	if remaining <= 0 {
		return nil
	}
	return rdb.Set(ctx, BlacklistKeyPrefix+jti, "1", remaining).Err()
}

// IsBlacklisted returns true if the jti has an active blacklist entry.
func IsBlacklisted(ctx context.Context, rdb *redis.Client, jti string) (bool, error) {
	n, err := rdb.Exists(ctx, BlacklistKeyPrefix+jti).Result()
	if err != nil {
		return false, fmt.Errorf("authmw: blacklist exists: %w", err)
	}
	return n > 0, nil
}

// ============================================================
// Epoch — user_blacklist_epoch:{user_id}
// ============================================================

// SetEpoch writes epoch = now+1 second for the given userID.
// Contract §3:
//
//	"iat has second resolution; a token minted in the same second must not
//	 survive the epoch write, so logout-all writes epoch = now + 1."
//
// The 30-day TTL is refreshed on every write (NFR-03 — no cleanup job needed).
func SetEpoch(ctx context.Context, rdb *redis.Client, userID string) error {
	epoch := time.Now().Unix() + 1
	return rdb.Set(ctx, EpochKeyPrefix+userID, epoch, epochTTL).Err()
}

// GetEpoch returns the stored epoch unix timestamp for userID.
// Returns 0 when no epoch exists (redis.Nil is treated as "no epoch set").
func GetEpoch(ctx context.Context, rdb *redis.Client, userID string) (int64, error) {
	val, err := rdb.Get(ctx, EpochKeyPrefix+userID).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("authmw: get epoch: %w", err)
	}
	return val, nil
}

// ============================================================
// Sessions — user_sessions:{user_id}  (hash, field = sid)
// ============================================================

// Session is the session record stored in user_sessions:{userID} under
// field sid. Contract §3.
type Session struct {
	SID         string    `json:"sid"`
	RefreshJTI  string    `json:"refresh_jti"`
	DeviceLabel string    `json:"device_label"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	CSRFToken   string    `json:"csrf_token"`
}

// StoreSession writes s into user_sessions:{userID} hash under field s.SID,
// and refreshes the hash's 30-day TTL.
func StoreSession(ctx context.Context, rdb *redis.Client, userID string, s Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("authmw: marshal session: %w", err)
	}
	key := SessionsKeyPrefix + userID
	if err := rdb.HSet(ctx, key, s.SID, string(data)).Err(); err != nil {
		return fmt.Errorf("authmw: hset session: %w", err)
	}
	return rdb.Expire(ctx, key, sessionTTL).Err()
}

// RotateSession updates RefreshJTI and LastSeenAt for an existing session,
// preserving SID and CSRFToken. Contract §3:
//
//	"Rotation updates refresh_jti and last_seen_at and preserves sid and csrf_token."
func RotateSession(ctx context.Context, rdb *redis.Client, userID string, s Session) error {
	return StoreSession(ctx, rdb, userID, s)
}

// GetSession retrieves one session record for a given SID.
// Returns (nil, redis.Nil) when not found.
func GetSession(ctx context.Context, rdb *redis.Client, userID, sid string) (*Session, error) {
	data, err := rdb.HGet(ctx, SessionsKeyPrefix+userID, sid).Bytes()
	if err != nil {
		return nil, err // includes redis.Nil
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("authmw: unmarshal session: %w", err)
	}
	return &s, nil
}

// DeleteSession removes one session field from the hash (single-device logout).
func DeleteSession(ctx context.Context, rdb *redis.Client, userID, sid string) error {
	return rdb.HDel(ctx, SessionsKeyPrefix+userID, sid).Err()
}

// AllSessions returns all session records stored for userID.
func AllSessions(ctx context.Context, rdb *redis.Client, userID string) ([]Session, error) {
	m, err := rdb.HGetAll(ctx, SessionsKeyPrefix+userID).Result()
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(m))
	for _, data := range m {
		var s Session
		if err := json.Unmarshal([]byte(data), &s); err != nil {
			return nil, fmt.Errorf("authmw: unmarshal session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// ============================================================
// Login fail counters — login_fail:{user_id}, login_fail_ip:{ip}
// ============================================================

// IncrLoginFailUser increments the per-account consecutive-failure counter
// with a sliding 15-minute TTL. Returns the new count.
func IncrLoginFailUser(ctx context.Context, rdb *redis.Client, userID string) (int64, error) {
	return incrSliding(ctx, rdb, LoginFailUserPrefix+userID, loginFailTTL)
}

// IncrLoginFailIP increments the per-IP consecutive-failure counter with a
// sliding 15-minute TTL. Returns the new count.
func IncrLoginFailIP(ctx context.Context, rdb *redis.Client, ip string) (int64, error) {
	return incrSliding(ctx, rdb, LoginFailIPPrefix+ip, loginFailTTL)
}

// GetLoginFailUser returns the current per-account failure count (0 if none).
func GetLoginFailUser(ctx context.Context, rdb *redis.Client, userID string) (int64, error) {
	return getCountOrZero(ctx, rdb, LoginFailUserPrefix+userID)
}

// ClearLoginFail deletes the per-account failure counter, the per-IP failure
// counter, and the backoff key. Contract §3:
//
//	"A successful login deletes all three keys."
func ClearLoginFail(ctx context.Context, rdb *redis.Client, userID, ip string) error {
	return rdb.Del(ctx,
		LoginFailUserPrefix+userID,
		LoginFailIPPrefix+ip,
		LoginBackoffPrefix+userID,
	).Err()
}

// ============================================================
// Login backoff — login_backoff:{user_id}
// ============================================================

// SetLoginBackoff writes backoff step n for userID. The TTL equals the delay
// for step n (BackoffDelay(n)), so the remaining TTL is what callers report
// as Retry-After. Contract §3:
//
//	"Value is the step n; TTL is the delay, 30 * 2^(n-1), capped at 3600."
func SetLoginBackoff(ctx context.Context, rdb *redis.Client, userID string, step int) error {
	delay := BackoffDelay(step)
	return rdb.Set(ctx, LoginBackoffPrefix+userID, step, delay).Err()
}

// GetLoginBackoffTTL returns the remaining backoff TTL for userID, which is
// the value to put in the Retry-After header. Returns 0 when not in backoff.
func GetLoginBackoffTTL(ctx context.Context, rdb *redis.Client, userID string) (time.Duration, error) {
	ttl, err := rdb.TTL(ctx, LoginBackoffPrefix+userID).Result()
	if err != nil {
		return 0, fmt.Errorf("authmw: backoff ttl: %w", err)
	}
	if ttl < 0 {
		return 0, nil // key absent or no TTL
	}
	return ttl, nil
}

// InBackoff returns true when the user currently has an active backoff lockout.
func InBackoff(ctx context.Context, rdb *redis.Client, userID string) (bool, error) {
	n, err := rdb.Exists(ctx, LoginBackoffPrefix+userID).Result()
	if err != nil {
		return false, fmt.Errorf("authmw: backoff exists: %w", err)
	}
	return n > 0, nil
}

// ============================================================
// helpers
// ============================================================

// incrSliding increments key and resets its TTL to ttl (sliding window).
func incrSliding(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) (int64, error) {
	pipe := rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("authmw: incr sliding %s: %w", key, err)
	}
	return incr.Val(), nil
}

// getCountOrZero returns the integer value of key, or 0 if the key is absent.
func getCountOrZero(ctx context.Context, rdb *redis.Client, key string) (int64, error) {
	val, err := rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("authmw: get count %s: %w", key, err)
	}
	return val, nil
}
