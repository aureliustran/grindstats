package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/libs/httpkit"
)

// rateLimitWindow is the fixed sliding-window duration for all rate limit
// buckets. The contract specifies per-minute limits (NFR-04).
const rateLimitWindow = time.Minute

// rateLimitRule describes one endpoint's rate limit bucket.
type rateLimitRule struct {
	// pathPrefix is matched as a prefix of c.Request.URL.Path.
	pathPrefix string
	// limit is the maximum allowed requests in the window.
	limit int64
	// bucketSuffix is used in the Redis key: ratelimit:{bucketSuffix}:{ip}.
	// Short, stable and human-readable for operational inspection.
	bucketSuffix string
}

// rateLimitRules lists every endpoint category the contract specifies, in
// the order they are evaluated. The first matching rule wins. Paths not
// matched by any rule are not rate-limited by this middleware.
//
// Contract §6 (NFR-04):
//   - login          10/min per IP (per-account enforcement is in the auth handler, FR-13)
//   - register       5/min per IP
//   - password-reset 5/min per IP (both /request and /confirm share the same bucket)
//   - verify-email   10/min per IP
//   - oauth/link/confirm 10/min per IP
//   - refresh        60/min per IP (per-session is approximated as per-IP at the
//     middleware layer; fine-grained enforcement is in the session handler)
var rateLimitRules = []rateLimitRule{
	{pathPrefix: "/api/v1/auth/login", limit: 10, bucketSuffix: "login"},
	{pathPrefix: "/api/v1/auth/register", limit: 5, bucketSuffix: "register"},
	{pathPrefix: "/api/v1/auth/password-reset", limit: 5, bucketSuffix: "pwreset"},
	{pathPrefix: "/api/v1/auth/verify-email", limit: 10, bucketSuffix: "verifyemail"},
	{pathPrefix: "/api/v1/auth/oauth/link/confirm", limit: 10, bucketSuffix: "oauthlinkconfirm"},
	{pathPrefix: "/api/v1/auth/refresh", limit: 60, bucketSuffix: "refresh"},
}

// RateLimit enforces per-endpoint, per-IP request rate limits (NFR-04) using
// Redis sliding-window counters. Exceeded requests receive 429 AUTH_RATE_LIMITED
// with a Retry-After header (seconds until the current window expires).
//
// Paths not in the configured rule set pass through without counting.
// A Redis I/O error is logged at warn and the request is allowed through
// (fail open for rate limiting — a temporary Redis blip must not deny all
// requests to rate-limited endpoints).
func RateLimit(rdb *redis.Client, logger *slog.Logger) gin.HandlerFunc {
	logger = logOr(logger)
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		rule := matchRule(path)
		if rule == nil {
			// Path not subject to rate limiting.
			c.Next()
			return
		}

		ip := c.ClientIP()
		key := authmw.RateLimitKeyPrefix + rule.bucketSuffix + ":" + ip
		ctx := c.Request.Context()

		count, retryAfter, err := checkAndIncrRateLimit(ctx, rdb, key)
		if err != nil {
			logger.WarnContext(ctx, "ratelimit: Redis error, failing open",
				"request_id", RequestIDFrom(c),
				"key", key,
				"error", err,
			)
			c.Next()
			return
		}

		if count > rule.limit {
			logger.InfoContext(ctx, "ratelimit: request rejected",
				"request_id", RequestIDFrom(c),
				"bucket", rule.bucketSuffix,
				"ip", ip,
				"count", count,
				"limit", rule.limit,
			)
			retrySeconds := int(retryAfter.Seconds())
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			c.Header("Retry-After", fmt.Sprintf("%d", retrySeconds))
			httpkit.Error(c, auditmodel.ErrAuthRateLimited)
			return
		}

		c.Next()
	}
}

// matchRule returns the first rule whose pathPrefix is a prefix of path,
// or nil when no rule matches.
func matchRule(path string) *rateLimitRule {
	for i := range rateLimitRules {
		if strings.HasPrefix(path, rateLimitRules[i].pathPrefix) {
			return &rateLimitRules[i]
		}
	}
	return nil
}

// checkAndIncrRateLimit increments the sliding-window counter for key and
// returns (newCount, ttl, nil). The TTL is set only when the key is new
// (EXPIRE ... NX) so a fixed window actually closes — refreshing it on every
// request would let a slow-but-steady caller keep the key alive forever and
// eventually trip the limit on cumulative count alone. The TTL after the
// increment is used as the Retry-After value so clients know when their
// current window resets.
func checkAndIncrRateLimit(
	ctx context.Context,
	rdb *redis.Client,
	key string,
) (count int64, retryAfter time.Duration, err error) {
	pipe := rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, rateLimitWindow)
	ttlCmd := pipe.TTL(ctx, key)
	if _, execErr := pipe.Exec(ctx); execErr != nil {
		return 0, 0, fmt.Errorf("ratelimit: pipeline exec: %w", execErr)
	}
	count = incrCmd.Val()

	ttl := ttlCmd.Val()
	if ttl < 0 {
		// Key has no TTL (unlikely after ExpireNX above) or Redis error;
		// fall back to the full window duration.
		ttl = rateLimitWindow
	}
	return count, ttl, nil
}

