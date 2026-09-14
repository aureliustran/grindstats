// Package hibp implements the Have I Been Pwned k-anonymity password check (D3).
//
// A hit → caller rejects the password with 400 VALIDATION_FAILED, rule:"breached".
// Unreachable / timeout → check is skipped (fail open); local dev and HIBP outages
// must never block signup (AUTH-001 story dependencies, D3).
// AUTH_HIBP_ENABLED=false → Client.IsBreached always returns (false, nil).
package hibp

import (
	"context"
	"crypto/sha1" //nolint:gosec // SHA-1 is required by the HIBP k-anonymity API spec
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client checks whether a password appears in the HIBP k-anonymity corpus.
type Client interface {
	// IsBreached returns true if the password hash suffix appears in the HIBP
	// response for its prefix. Returns a non-nil error only when the HIBP
	// endpoint was unreachable or returned an unexpected status — callers treat
	// any error as "skip the check" (D3).
	IsBreached(ctx context.Context, password string) (bool, error)
}

// HTTPClient is the live HIBP k-anonymity client. Use New to construct it.
type HTTPClient struct {
	httpClient *http.Client
	enabled    bool
	baseURL    string // defaults to defaultBaseURL; overridable via NewWithURL for tests
}

// New creates an HTTPClient. When enabled is false, IsBreached always returns
// (false, nil) — used by the compose file (AUTH_HIBP_ENABLED=false) and tests.
// timeout is the per-request deadline; D3 specifies the default as 2 seconds.
func New(enabled bool, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		httpClient: &http.Client{Timeout: timeout},
		enabled:    enabled,
	}
}

// baseURL is the HIBP range endpoint prefix. Overridable for tests.
var defaultBaseURL = "https://api.pwnedpasswords.com/range"

// NewWithURL creates an HTTPClient that hits baseURL instead of the live HIBP
// endpoint. Used in tests to point at a local httptest.Server.
func NewWithURL(enabled bool, timeout time.Duration, baseURL string) *HTTPClient {
	c := New(enabled, timeout)
	c.baseURL = baseURL
	return c
}

// IsBreached sends the first 5 hex chars of the SHA-1 of password to
// api.pwnedpasswords.com/range and reports whether the suffix appears in the
// returned list. Returns (false, err) for network or HTTP errors; callers skip
// the check on any error (D3 fail-open).
func (c *HTTPClient) IsBreached(ctx context.Context, password string) (bool, error) {
	if !c.enabled {
		return false, nil
	}

	// k-anonymity: send only the first 5 hex chars of the SHA-1 hash.
	h := sha1.Sum([]byte(password)) //nolint:gosec
	hex := fmt.Sprintf("%X", h[:])
	prefix := hex[:5]
	suffix := hex[5:]

	base := c.baseURL
	if base == "" {
		base = defaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/"+prefix, nil)
	if err != nil {
		return false, fmt.Errorf("hibp: build request: %w", err)
	}
	req.Header.Set("Add-Padding", "true") // prevents response-size side-channel

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("hibp: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("hibp: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("hibp: read body: %w", err)
	}

	// Each line: "<35-char-suffix>:<count>\r\n"
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 36 { // 35 suffix chars + ":" + at least one digit
			continue
		}
		if !strings.EqualFold(line[:35], suffix) {
			continue
		}
		// Match found; check the count — a "0" entry is padding, not a breach.
		rest := strings.SplitN(line, ":", 2)
		if len(rest) == 2 && rest[1] != "0" {
			return true, nil
		}
	}
	return false, nil
}
