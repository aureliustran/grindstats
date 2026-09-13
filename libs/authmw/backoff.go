package authmw

import "time"

// BackoffDelay returns the lockout duration for backoff step n.
//
// Contract §3 (FR-13):
//
//	"30 * 2^(n-1) seconds, capped at 3600."
//
// Step 1 → 30 s, step 2 → 60 s, step 3 → 120 s, …, step 7+ → 3600 s.
//
// This is a pure function — callers use it both to set the Redis TTL and to
// compute the Retry-After header value, so there is exactly one place where
// the sequence is defined.
func BackoffDelay(step int) time.Duration {
	if step < 1 {
		step = 1
	}
	const cap = 3600
	seconds := 30
	for i := 1; i < step; i++ {
		seconds *= 2
		if seconds >= cap {
			seconds = cap
			break
		}
	}
	if seconds > cap {
		seconds = cap
	}
	return time.Duration(seconds) * time.Second
}
