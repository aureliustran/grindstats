package authmw

import "net/http"

// Cookie names (contract §2.2).
const (
	AccessCookieName     = "gs_access"
	RefreshCookieName    = "gs_refresh"
	OAuthStateCookieName = "gs_oauth_state"
)

// Cookie paths (contract §2.2).
const (
	accessCookiePath     = "/"
	refreshCookiePath    = "/api/v1/auth"
	oauthStateCookiePath = "/api/v1/auth/oauth"
)

// Cookie Max-Age values (contract §2.2, in seconds).
const (
	accessCookieMaxAge     = 900       // 15 minutes
	refreshCookieMaxAge    = 2592000   // 30 days
	oauthStateCookieMaxAge = 600       // 10 minutes
)

// CookieOptions controls per-deployment cookie behaviour.
type CookieOptions struct {
	// Secure controls whether the Secure attribute is emitted.
	// Default (and production) value is true. Set false only when
	// AUTH_COOKIE_SECURE=false for local plain-http development.
	Secure bool
}

// DefaultCookieOptions returns options with Secure=true. This is what all
// production paths and tests asserting cookie attributes should use.
func DefaultCookieOptions() CookieOptions {
	return CookieOptions{Secure: true}
}

// SetAccessCookie writes the gs_access JWT cookie.
// Contract §2.2: Path=/, HttpOnly, Secure, SameSite=Lax, Max-Age=900.
func SetAccessCookie(w http.ResponseWriter, token string, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessCookieName,
		Value:    token,
		Path:     accessCookiePath,
		MaxAge:   accessCookieMaxAge,
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// SetRefreshCookie writes the gs_refresh JWT cookie.
// Contract §2.2: Path=/api/v1/auth, HttpOnly, Secure, SameSite=Lax, Max-Age=2592000.
// The narrow Path keeps the refresh cookie off every endpoint other than the
// auth sub-tree, which is part of the replay-protection scheme (contract D7).
func SetRefreshCookie(w http.ResponseWriter, token string, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		MaxAge:   refreshCookieMaxAge,
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// SetSessionCookies is a convenience that writes both token cookies at once.
func SetSessionCookies(w http.ResponseWriter, accessToken, refreshToken string, opts CookieOptions) {
	SetAccessCookie(w, accessToken, opts)
	SetRefreshCookie(w, refreshToken, opts)
}

// ClearCookies writes both session cookies with MaxAge=-1 (delete instruction).
func ClearCookies(w http.ResponseWriter, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessCookieName,
		Value:    "",
		Path:     accessCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// SetOAuthStateCookie writes the gs_oauth_state cookie used during the OAuth
// authorisation code flow.
// Contract §2.2: Path=/api/v1/auth/oauth, HttpOnly, Secure, SameSite=Lax, Max-Age=600.
func SetOAuthStateCookie(w http.ResponseWriter, value string, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthStateCookieName,
		Value:    value,
		Path:     oauthStateCookiePath,
		MaxAge:   oauthStateCookieMaxAge,
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}
