package session_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/argon2"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/libs/authmw"
	"grindstats/services/monolith/internal/auth/authdomain"
	"grindstats/services/monolith/internal/auth/session"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── key material ─────────────────────────────────────────────────────────────

func newTestKeySet(t *testing.T) (*authmw.KeySet, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("newTestKeySet: %v", err)
	}
	ks := authmw.NewKeySetFromMemory("kid1", priv, map[string]*rsa.PublicKey{"kid1": &priv.PublicKey})
	return ks, priv
}

// ─── Redis ────────────────────────────────────────────────────────────────────

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

// ─── AccountRepo fake ─────────────────────────────────────────────────────────

type fakeAccountRepo struct {
	byEmail map[string]*authdomain.Account
	byID    map[uuid.UUID]*authdomain.Account
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{
		byEmail: make(map[string]*authdomain.Account),
		byID:    make(map[uuid.UUID]*authdomain.Account),
	}
}

func (r *fakeAccountRepo) add(a authdomain.Account) {
	r.byEmail[a.EmailLower] = &a
	r.byID[a.ID] = &a
}

func (r *fakeAccountRepo) ByEmail(_ context.Context, emailLower string) (*authdomain.Account, error) {
	a := r.byEmail[emailLower]
	if a == nil {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAccountRepo) ByID(_ context.Context, id uuid.UUID) (*authdomain.Account, error) {
	a := r.byID[id]
	if a == nil {
		return nil, authdomain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAccountRepo) Create(_ context.Context, _ authdomain.Account) error {
	panic("fakeAccountRepo.Create not used in session tests")
}

func (r *fakeAccountRepo) SetPasswordHash(_ context.Context, _ uuid.UUID, _ string) error {
	panic("fakeAccountRepo.SetPasswordHash not used in session tests")
}

func (r *fakeAccountRepo) MarkEmailVerified(_ context.Context, _ uuid.UUID, _ time.Time) error {
	panic("fakeAccountRepo.MarkEmailVerified not used in session tests")
}

// ─── password helpers ─────────────────────────────────────────────────────────

// mustArgon2Hash returns a valid PHC-format argon2id hash of password.
// Uses reduced parameters (t=1) for test speed while retaining the correct
// format that verifyArgon2ID can parse.
func mustArgon2Hash(password string) string {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic("mustArgon2Hash: rand.Read: " + err.Error())
	}
	// t=1 for speed in tests; production uses t=3 (be-auth-credentials' choice)
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

// newTestAccount returns an active, unverified account whose password is hash
// of the given plaintext.
func newTestAccount(email, password string) authdomain.Account {
	return authdomain.Account{
		ID:           uuid.New(),
		Email:        email,
		EmailLower:   email,
		PasswordHash: mustArgon2Hash(password),
		Role:         auditmodel.RoleUser,
		Status:       auditmodel.AccountStatusActive,
	}
}

// ─── handler builder ──────────────────────────────────────────────────────────

type testDeps struct {
	ks      *authmw.KeySet
	mr      *miniredis.Miniredis
	rdb     *redis.Client
	repo    *fakeAccountRepo
	audit   *auditlog.Fake
	handler *session.Handler
}

func newTestDeps(t *testing.T) *testDeps {
	t.Helper()
	ks, _ := newTestKeySet(t)
	mr, rdb := newTestRedis(t)
	repo := newFakeAccountRepo()
	fake := auditlog.NewFake()
	h := session.New(ks, rdb, repo, fake, authmw.DefaultCookieOptions(), nil)
	return &testDeps{ks: ks, mr: mr, rdb: rdb, repo: repo, audit: fake, handler: h}
}

// router returns a gin.Engine with the handler's routes registered.
func (d *testDeps) router() *gin.Engine {
	r := gin.New()
	d.handler.Register(r)
	return r
}

// routerWithClaims returns a gin.Engine that pre-injects claims into every
// request context, simulating the gateway auth middleware.
func (d *testDeps) routerWithClaims(claims authmw.Claims) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			authmw.WithClaims(c.Request.Context(), claims),
		)
		c.Next()
	})
	d.handler.Register(r)
	return r
}

// mintAccess mints a live access token for a, returning the cookie string and
// the Claims struct the handler will see in context.
func (d *testDeps) mintAccess(t *testing.T, a authdomain.Account) (tokenStr string, claims authmw.Claims) {
	t.Helper()
	sid := uuid.New().String()
	tok, jti, err := d.ks.MintAccess(a.ID.String(), sid, string(a.Role), "free", a.IsVerified())
	if err != nil {
		t.Fatalf("mintAccess: %v", err)
	}
	return tok, authmw.Claims{
		Sub:           a.ID.String(),
		SID:           sid,
		JTI:           jti,
		IAT:           time.Now().Unix(),
		Typ:           authmw.TypAccess,
		Role:          string(a.Role),
		Tier:          "free",
		EmailVerified: a.IsVerified(),
	}
}

// mintRefresh mints a live refresh token for a and sid.
func (d *testDeps) mintRefresh(t *testing.T, sub, sid string) (tokenStr, jti string) {
	t.Helper()
	tok, jtiOut, err := d.ks.MintRefresh(sub, sid)
	if err != nil {
		t.Fatalf("mintRefresh: %v", err)
	}
	return tok, jtiOut
}

// storeSession writes a session record to Redis.
func (d *testDeps) storeSession(t *testing.T, userID, sid, refreshJTI, csrfToken string) {
	t.Helper()
	now := time.Now()
	sess := authmw.Session{
		SID:        sid,
		RefreshJTI: refreshJTI,
		CreatedAt:  now,
		LastSeenAt: now,
		CSRFToken:  csrfToken,
	}
	if err := authmw.StoreSession(context.Background(), d.rdb, userID, sess); err != nil {
		t.Fatalf("storeSession: %v", err)
	}
}

// storeSessionAt writes a session record with a specific LastSeenAt time.
func (d *testDeps) storeSessionAt(t *testing.T, userID, sid, refreshJTI, csrfToken string, lastSeenAt time.Time) {
	t.Helper()
	sess := authmw.Session{
		SID:        sid,
		RefreshJTI: refreshJTI,
		CreatedAt:  lastSeenAt,
		LastSeenAt: lastSeenAt,
		CSRFToken:  csrfToken,
	}
	if err := authmw.StoreSession(context.Background(), d.rdb, userID, sess); err != nil {
		t.Fatalf("storeSessionAt: %v", err)
	}
}

// addCookie attaches a named cookie to the request.
func addCookie(req *http.Request, name, value string) {
	req.AddCookie(&http.Cookie{Name: name, Value: value})
}

// generateCSRF creates a random CSRF token for test setup.
func generateCSRF() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic("generateCSRF: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
