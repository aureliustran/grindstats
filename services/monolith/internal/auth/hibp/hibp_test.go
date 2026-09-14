package hibp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"grindstats/services/monolith/internal/auth/hibp"
)

// Scenario: a breached password is rejected or flagged — HIBP hit
// TC-04: HIBP returns a matching suffix → IsBreached returns true.
func TestIsBreached_hit(t *testing.T) {
	// SHA-1("password") = 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8
	// prefix = "5BAA6", suffix = "1E4C9B93F3F0682250B6CF8331B7EE68FD8"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a response containing the suffix for "password"
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1E4C9B93F3F0682250B6CF8331B7EE68FD8:3303003\r\n"))
	}))
	defer srv.Close()

	c := hibp.NewWithURL(true, 2*time.Second, srv.URL)
	breached, err := c.IsBreached(context.Background(), "password")
	require.NoError(t, err)
	assert.True(t, breached, "expected password to be reported as breached")
}

// Scenario: a breached password is rejected or flagged — HIBP miss
// TC-05: HIBP returns a response that does not contain the suffix → IsBreached returns false.
func TestIsBreached_miss(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No matching suffix in the response
		_, _ = w.Write([]byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:1\r\n"))
	}))
	defer srv.Close()

	c := hibp.NewWithURL(true, 2*time.Second, srv.URL)
	breached, err := c.IsBreached(context.Background(), "password")
	require.NoError(t, err)
	assert.False(t, breached, "expected password NOT to be reported as breached")
}

// Scenario: a breached password is rejected or flagged — HIBP unreachable → allowed with a warn
// TC-04 (unreachable): when the endpoint is unreachable IsBreached returns an error,
// which the caller treats as "skip the check" (D3 fail-open).
func TestIsBreached_unreachable(t *testing.T) {
	// Point at a server that immediately closes the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force an error by hijacking and closing
		panic("should not be reached — server closed")
	}))
	srv.Close() // close immediately so requests fail

	c := hibp.NewWithURL(true, 2*time.Second, srv.URL)
	breached, err := c.IsBreached(context.Background(), "password")
	assert.Error(t, err, "expected an error for unreachable server")
	assert.False(t, breached, "breached must be false on error")
}

// Scenario: AUTH_HIBP_ENABLED=false disables the check entirely.
func TestIsBreached_disabled(t *testing.T) {
	c := hibp.New(false, 2*time.Second)
	breached, err := c.IsBreached(context.Background(), "any-password")
	require.NoError(t, err)
	assert.False(t, breached)
}

// Scenario: a count of "0" in the response is padding, not a breach.
func TestIsBreached_paddingEntryNotBreached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1E4C9B93F3F0682250B6CF8331B7EE68FD8:0\r\n"))
	}))
	defer srv.Close()

	c := hibp.NewWithURL(true, 2*time.Second, srv.URL)
	breached, err := c.IsBreached(context.Background(), "password")
	require.NoError(t, err)
	assert.False(t, breached, "a count of 0 is padding — must not be treated as breached")
}
