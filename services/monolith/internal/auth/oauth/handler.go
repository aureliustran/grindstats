// Package oauth implements the Google OAuth2 authorize and callback endpoints
// (contract §1, rows 5–6): authorization-code + PKCE flow, state validation,
// account provisioning via authdomain.AccountProvisioner, and the §4.3 linking
// proof flow (ErrLinkRequired → email token → redirect).
//
// This package imports no peer wave-3 package. It reaches credentials logic
// only through authdomain.AccountProvisioner.
package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	goauth2google "golang.org/x/oauth2/google"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/httpkit"
	"grindstats/services/monolith/internal/auth/authdomain"
	"grindstats/services/monolith/internal/auth/mailer"
)

const (
	stateCookieName = "gs_oauth_state"
	stateCookiePath = "/api/v1/auth/oauth"
	stateCookieTTL  = 600 // 10 minutes, contract §2.2
)

// Config holds the OAuth provider configuration injected at the wave-4
// composition root. StateKey must be kept secret; it signs the state cookie.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	StateKey     []byte // HMAC-SHA256 key; treat as a secret
	CookieSecure bool   // false only for plain-http local dev (contract §2.2)
}

// Handler implements the Google OAuth authorize (#5) and callback (#6) endpoints.
type Handler struct {
	oauth2Cfg    oauth2.Config
	stateKey     []byte
	cookieSecure bool

	provisioner authdomain.AccountProvisioner
	accounts    authdomain.AccountRepo
	tokens      authdomain.LinkTokenRepo
	session     authdomain.SessionIssuer
	mailer      mailer.Mailer
	audit       auditlog.Writer

	// userInfoURL is the Google userinfo endpoint. Overridable in tests.
	userInfoURL string
	// httpClient is injected into the oauth2 context for tests. Nil in production.
	httpClient *http.Client
}

// New creates a Handler.
func New(
	cfg Config,
	provisioner authdomain.AccountProvisioner,
	accounts authdomain.AccountRepo,
	tokens authdomain.LinkTokenRepo,
	session authdomain.SessionIssuer,
	m mailer.Mailer,
	aud auditlog.Writer,
) *Handler {
	return &Handler{
		oauth2Cfg: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     goauth2google.Endpoint,
		},
		stateKey:     cfg.StateKey,
		cookieSecure: cfg.CookieSecure,
		provisioner:  provisioner,
		accounts:     accounts,
		tokens:       tokens,
		session:      session,
		mailer:       m,
		audit:        aud,
		userInfoURL:  "https://www.googleapis.com/oauth2/v3/userinfo",
	}
}

// Register mounts the OAuth endpoints onto r. Called by the wave-4 root.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/auth/oauth/google", h.authorize)
	r.GET("/auth/oauth/google/callback", h.callback)
}

// SetTestEndpoints overrides the OAuth2 provider endpoints so tests can point
// at a local httptest.Server instead of Google. Not for production use.
func (h *Handler) SetTestEndpoints(tokenURL, authURL, userInfoURL string) {
	h.oauth2Cfg.Endpoint = oauth2.Endpoint{
		TokenURL: tokenURL,
		AuthURL:  authURL,
	}
	h.userInfoURL = userInfoURL
}

// SetTestHTTPClient injects a custom *http.Client into the oauth2 context so
// the exchange and userinfo calls go through the mock server's transport.
// Not for production use.
func (h *Handler) SetTestHTTPClient(c *http.Client) {
	h.httpClient = c
}

// ─── Authorize ────────────────────────────────────────────────────────────────

// authorize handles GET /auth/oauth/google — generates state+PKCE, stores them
// in a signed HttpOnly cookie, and redirects to Google's consent screen.
func (h *Handler) authorize(c *gin.Context) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		httpkit.InternalError(c)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		httpkit.InternalError(c)
		return
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	cookieValue, err := h.signState(state, verifier)
	if err != nil {
		httpkit.InternalError(c)
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     stateCookieName,
		Value:    cookieValue,
		Path:     stateCookiePath,
		MaxAge:   stateCookieTTL,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// PKCE S256 code challenge
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authURL := h.oauth2Cfg.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	c.Redirect(http.StatusFound, authURL)
}

// ─── Callback ─────────────────────────────────────────────────────────────────

// callback handles GET /auth/oauth/google/callback.
// All error paths redirect — never a JSON body (contract §1 row 6).
func (h *Handler) callback(c *gin.Context) {
	// User denied the consent screen.
	if c.Query("error") == "access_denied" {
		c.Redirect(http.StatusFound, "/?auth_error=oauth_cancelled")
		return
	}

	// Validate state cookie + PKCE verifier.
	cookieValue, err := c.Cookie(stateCookieName)
	if err != nil || cookieValue == "" {
		c.Redirect(http.StatusFound, "/?auth_error=oauth_failed")
		return
	}
	_, verifier, err := h.verifyState(cookieValue, c.Query("state"))
	if err != nil {
		c.Redirect(http.StatusFound, "/?auth_error=oauth_failed")
		return
	}

	// Clear the state cookie — it is single-use.
	http.SetCookie(c.Writer, &http.Cookie{
		Name:   stateCookieName,
		Path:   stateCookiePath,
		MaxAge: -1,
	})

	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusFound, "/?auth_error=oauth_failed")
		return
	}

	// Exchange code for token, with optional test HTTP client injected via context.
	ctx := c.Request.Context()
	if h.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, h.httpClient)
	}

	token, err := h.oauth2Cfg.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
	)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "oauth: exchange code", "error", err)
		c.Redirect(http.StatusFound, "/?auth_error=oauth_failed")
		return
	}

	// Fetch Google user info.
	gUser, err := h.fetchGoogleUser(ctx, token)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "oauth: fetch google user", "error", err)
		c.Redirect(http.StatusFound, "/?auth_error=oauth_failed")
		return
	}

	// Provision (or look up) the account.
	acc, err := h.provisioner.ProvisionFromOAuth(ctx,
		auditmodel.LinkedProviderGoogle, gUser.Sub, gUser.Email)
	if err != nil {
		if errors.Is(err, authdomain.ErrLinkRequired) {
			// §4.3: email a linking-proof token and redirect.
			if linkErr := h.handleLinkRequired(c, gUser); linkErr != nil {
				slog.ErrorContext(c.Request.Context(), "oauth: handleLinkRequired", "error", linkErr)
			}
			c.Redirect(http.StatusFound, "/?auth_error=oauth_link_required")
			return
		}
		slog.ErrorContext(c.Request.Context(), "oauth: provision", "error", err)
		c.Redirect(http.StatusFound, "/?auth_error=oauth_failed")
		return
	}

	// Issue session — sets both cookies on the response writer.
	if _, err := h.session.Issue(ctx, c.Writer, *acc,
		c.GetHeader("User-Agent"), c.ClientIP()); err != nil {
		slog.ErrorContext(c.Request.Context(), "oauth: issue session", "error", err)
		c.Redirect(http.StatusFound, "/?auth_error=oauth_failed")
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

// handleLinkRequired issues an oauth_link token, emails it to the existing
// account's address, and writes the auth.oauth.link_required audit event.
// Called only when ProvisionFromOAuth returns ErrLinkRequired (contract §4.3).
func (h *Handler) handleLinkRequired(c *gin.Context, gUser *googleUserInfo) error {
	acc, err := h.accounts.ByEmail(c.Request.Context(), strings.ToLower(gUser.Email))
	if err != nil {
		return fmt.Errorf("oauth: look up account for link: %w", err)
	}
	if acc == nil {
		return fmt.Errorf("oauth: account not found for link email %q", gUser.Email)
	}

	payload := struct {
		Provider string `json:"provider"`
		Subject  string `json:"subject"`
		Email    string `json:"email"`
	}{
		Provider: string(auditmodel.LinkedProviderGoogle),
		Subject:  gUser.Sub,
		Email:    gUser.Email,
	}

	rawToken, err := h.tokens.Issue(c.Request.Context(), acc.ID,
		auditmodel.LinkKindOauthLink, time.Hour, payload)
	if err != nil {
		return fmt.Errorf("oauth: issue link token: %w", err)
	}

	if err := h.mailer.SendOAuthLink(c.Request.Context(), acc.Email, rawToken); err != nil {
		slog.ErrorContext(c.Request.Context(), "oauth: send link email", "error", err)
		// non-fatal — the redirect still happens
	}

	uid := acc.ID
	_ = h.audit.Write(c.Request.Context(), auditmodel.EvtAuthOauthLinkRequired,
		map[string]any{
			"user_id":   uid.String(),
			"provider":  auditmodel.LinkedProviderGoogle,
			"source_ip": c.ClientIP(),
		},
		auditlog.WriteContext{},
	)

	return nil
}

// ─── State / PKCE cookie helpers ─────────────────────────────────────────────

type oauthStateCookie struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
}

// signState encodes state+verifier as a signed cookie value:
//
//	base64url(json) + "." + base64url(HMAC-SHA256)
func (h *Handler) signState(state, verifier string) (string, error) {
	data, err := json.Marshal(oauthStateCookie{State: state, Verifier: verifier})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, h.stateKey)
	if _, err := mac.Write([]byte(encoded)); err != nil {
		return "", err
	}
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + sig, nil
}

// verifyState validates the cookie's HMAC and confirms the embedded state
// matches the queryState parameter from Google. Returns (state, verifier, err).
func (h *Handler) verifyState(cookieValue, queryState string) (string, string, error) {
	dot := strings.LastIndex(cookieValue, ".")
	if dot < 0 {
		return "", "", errors.New("oauth: invalid cookie: no signature delimiter")
	}
	encoded := cookieValue[:dot]
	sig := cookieValue[dot+1:]

	mac := hmac.New(sha256.New, h.stateKey)
	if _, err := mac.Write([]byte(encoded)); err != nil {
		return "", "", err
	}
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", "", errors.New("oauth: cookie signature mismatch")
	}

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("oauth: decode cookie: %w", err)
	}

	var sc oauthStateCookie
	if err := json.Unmarshal(data, &sc); err != nil {
		return "", "", fmt.Errorf("oauth: parse cookie: %w", err)
	}

	if sc.State != queryState {
		return "", "", errors.New("oauth: state mismatch")
	}

	return sc.State, sc.Verifier, nil
}

// ─── Google userinfo ──────────────────────────────────────────────────────────

type googleUserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

// fetchGoogleUser calls the Google userinfo endpoint using the oauth2 token's
// HTTP client (which injects Authorization: Bearer automatically).
func (h *Handler) fetchGoogleUser(ctx context.Context, token *oauth2.Token) (*googleUserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	resp, err := client.Get(h.userInfoURL)
	if err != nil {
		return nil, fmt.Errorf("oauth: userinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth: userinfo status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oauth: read userinfo: %w", err)
	}

	var info googleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("oauth: parse userinfo: %w", err)
	}
	if info.Sub == "" || info.Email == "" {
		return nil, errors.New("oauth: missing sub or email in userinfo response")
	}
	return &info, nil
}
