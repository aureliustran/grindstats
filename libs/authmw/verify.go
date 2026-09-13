package authmw

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"grindstats/libs/auditmodel"
)

// Check validates tokenStr against the four-step chain defined in contract §6:
//
//	signature → expiry → blacklist:{jti} → iat vs user_blacklist_epoch:{sub}
//
// It returns the decoded Claims on success (reason == "", err == nil).
// On the first failing check it returns the reason and nil error — the caller
// records that in an audit event and maps it to AUTH_INVALID_TOKEN in the
// response; it must not leak which check failed.
// On a Redis I/O error it returns ("", err) with no reason; the caller applies
// the degradation policy (CheckDegradation) to decide 503 vs fail-open.
//
// expectedTyp must be TypAccess or TypRefresh. A token whose "typ" claim
// does not match expectedTyp is rejected as BadSignature — the token does
// not serve the presented purpose regardless of its cryptographic validity.
// This check runs after signature so that the "typ" value in the payload can
// be trusted.
//
// Pass a nil rdb only in unit tests that isolate the signature/expiry path;
// production callers always supply a client (real or miniredis).
func (ks *KeySet) Check(
	ctx context.Context,
	rdb *redis.Client,
	tokenStr,
	expectedTyp string,
) (Claims, auditmodel.TokenRejectReason, error) {
	// --- Step 1: Signature (and structural validity) ---
	//
	// golang-jwt/v5 verifies the signature first. If the signature is invalid
	// the claim validator never runs, so expiry and other standard-claim errors
	// only surface when the signature was good. This naturally implements the
	// specified check order.
	var mapClaims jwt.MapClaims
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))

	_, parseErr := parser.ParseWithClaims(tokenStr, &mapClaims, ks.keyFunc)

	if parseErr != nil {
		// Distinguish expiry (signature was OK, standard-claim check failed)
		// from a bad/unknown signature.
		if errors.Is(parseErr, jwt.ErrTokenExpired) &&
			!errors.Is(parseErr, jwt.ErrTokenSignatureInvalid) &&
			!errors.Is(parseErr, jwt.ErrTokenUnverifiable) &&
			!errors.Is(parseErr, jwt.ErrTokenMalformed) {
			// --- Step 2: Expiry ---
			return Claims{}, auditmodel.TokenRejectReasonExpired, nil
		}
		return Claims{}, auditmodel.TokenRejectReasonBadSignature, nil
	}

	// Signature and expiry both passed; extract the claims.
	c, err := extractClaims(mapClaims)
	if err != nil {
		return Claims{}, auditmodel.TokenRejectReasonBadSignature,
			fmt.Errorf("authmw: extract claims: %w", err)
	}

	// typ check — reject wrong-kind tokens before Redis round-trips.
	// A refresh token presented as access (or vice-versa) is a BadSignature
	// because the token does not prove the asserted identity for this purpose.
	if c.Typ != expectedTyp {
		return Claims{}, auditmodel.TokenRejectReasonBadSignature, nil
	}

	if rdb == nil {
		// Test-only path; skip Redis checks.
		return c, "", nil
	}

	// --- Step 3: Blacklist ---
	blacklisted, err := IsBlacklisted(ctx, rdb, c.JTI)
	if err != nil {
		return Claims{}, "", fmt.Errorf("authmw: blacklist check: %w", err)
	}
	if blacklisted {
		return Claims{}, auditmodel.TokenRejectReasonBlacklisted, nil
	}

	// --- Step 4: Epoch ---
	epoch, err := GetEpoch(ctx, rdb, c.Sub)
	if err != nil {
		return Claims{}, "", fmt.Errorf("authmw: epoch check: %w", err)
	}
	// epoch == 0 means no logout-all was ever issued for this user.
	// epoch > 0 means any token whose iat is strictly before the epoch is stale.
	// Contract §3: epoch is written as now+1 so that a token minted in the same
	// second does not survive the epoch write.
	if epoch > 0 && c.IAT < epoch {
		return Claims{}, auditmodel.TokenRejectReasonEpochStale, nil
	}

	return c, "", nil
}
