package oauth_test

// testfakes_test.go — in-memory fakes for the oauth package tests.

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// ─── fakeAccountRepo ──────────────────────────────────────────────────────────

type fakeAccountRepo struct {
	mu      sync.Mutex
	byEmail map[string]*authdomain.Account
	byID    map[uuid.UUID]*authdomain.Account
	created []authdomain.Account
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{
		byEmail: make(map[string]*authdomain.Account),
		byID:    make(map[uuid.UUID]*authdomain.Account),
	}
}

func (r *fakeAccountRepo) seed(a authdomain.Account) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := a
	r.byEmail[a.EmailLower] = &cp
	r.byID[a.ID] = &cp
}

func (r *fakeAccountRepo) ByEmail(_ context.Context, emailLower string) (*authdomain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.byEmail[emailLower]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeAccountRepo) ByID(_ context.Context, id uuid.UUID) (*authdomain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.byID[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, authdomain.ErrNotFound
}

func (r *fakeAccountRepo) Create(_ context.Context, a authdomain.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byEmail[a.EmailLower]; exists {
		return authdomain.ErrEmailTaken
	}
	cp := a
	r.byEmail[a.EmailLower] = &cp
	r.byID[a.ID] = &cp
	r.created = append(r.created, cp)
	return nil
}

func (r *fakeAccountRepo) SetPasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return authdomain.ErrNotFound
	}
	a.PasswordHash = hash
	return nil
}

func (r *fakeAccountRepo) MarkEmailVerified(_ context.Context, id uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return authdomain.ErrNotFound
	}
	if a.EmailVerifiedAt == nil {
		a.EmailVerifiedAt = &at
	}
	return nil
}

// ─── fakeOAuthIdentityRepo ────────────────────────────────────────────────────

type fakeOAuthIdentityRepo struct {
	mu    sync.Mutex
	links map[string]*authdomain.Account // key: provider+":"+subject
	calls []linkCall
}

type linkCall struct {
	userID   uuid.UUID
	provider auditmodel.LinkedProvider
	subject  string
	email    string
}

func newFakeOAuthIdentityRepo() *fakeOAuthIdentityRepo {
	return &fakeOAuthIdentityRepo{links: make(map[string]*authdomain.Account)}
}

func (r *fakeOAuthIdentityRepo) seedLinked(provider auditmodel.LinkedProvider, subject string, acc authdomain.Account) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := string(provider) + ":" + subject
	cp := acc
	r.links[key] = &cp
}

func (r *fakeOAuthIdentityRepo) BySubject(_ context.Context, provider auditmodel.LinkedProvider, subject string) (*authdomain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := string(provider) + ":" + subject
	if a, ok := r.links[key]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, authdomain.ErrNotFound
}

func (r *fakeOAuthIdentityRepo) Link(_ context.Context, userID uuid.UUID, provider auditmodel.LinkedProvider, subject, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, linkCall{userID: userID, provider: provider, subject: subject, email: email})
	return nil
}

// ─── fakeLinkTokenRepo ────────────────────────────────────────────────────────

type storedToken struct {
	userID  uuid.UUID
	kind    auditmodel.LinkKind
	raw     string
	used    bool
	payload []byte
}

type fakeLinkTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]*storedToken
	issued []string
}

func newFakeLinkTokenRepo() *fakeLinkTokenRepo {
	return &fakeLinkTokenRepo{tokens: make(map[string]*storedToken)}
}

func (r *fakeLinkTokenRepo) Issue(_ context.Context, userID uuid.UUID, kind auditmodel.LinkKind, _ time.Duration, payload any) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw := uuid.New().String()
	var payloadBytes []byte
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return "", err
		}
	}
	r.tokens[raw] = &storedToken{userID: userID, kind: kind, raw: raw, payload: payloadBytes}
	r.issued = append(r.issued, raw)
	return raw, nil
}

func (r *fakeLinkTokenRepo) Consume(_ context.Context, kind auditmodel.LinkKind, raw string) (*authdomain.LinkToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.tokens[raw]
	if !ok {
		return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonUnknown}
	}
	if st.used {
		return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonAlreadyConsumed}
	}
	if st.kind != kind {
		return nil, &authdomain.LinkInvalidError{Reason: auditmodel.LinkRejectReasonUnknown}
	}
	st.used = true
	return &authdomain.LinkToken{
		ID:      uuid.New(),
		UserID:  st.userID,
		Kind:    kind,
		Payload: st.payload,
	}, nil
}

// ─── fakeSessionIssuer ────────────────────────────────────────────────────────

type fakeSessionIssuer struct {
	mu       sync.Mutex
	issueCnt int
	revokeCnt int
}

func (s *fakeSessionIssuer) Issue(_ context.Context, _ http.ResponseWriter, _ authdomain.Account, _, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.issueCnt++
	return "csrf-fake-token", nil
}

func (s *fakeSessionIssuer) RevokeAll(_ context.Context, _ uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokeCnt++
	return nil
}

// ─── fakeMailer ───────────────────────────────────────────────────────────────

type fakeMailer struct {
	mu         sync.Mutex
	oauthLinks []string
}

func (m *fakeMailer) SendVerification(_ context.Context, _, _ string) error { return nil }
func (m *fakeMailer) SendPasswordReset(_ context.Context, _, _ string) error { return nil }
func (m *fakeMailer) SendOAuthLink(_ context.Context, _, rawToken string) error {
	m.mu.Lock()
	m.oauthLinks = append(m.oauthLinks, rawToken)
	m.mu.Unlock()
	return nil
}

// compile-time interface checks
var _ authdomain.AccountRepo = (*fakeAccountRepo)(nil)
var _ authdomain.OAuthIdentityRepo = (*fakeOAuthIdentityRepo)(nil)
var _ authdomain.LinkTokenRepo = (*fakeLinkTokenRepo)(nil)
var _ authdomain.SessionIssuer = (*fakeSessionIssuer)(nil)
