package credentials

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
)

// Provisioner implements authdomain.AccountProvisioner. It is constructed by
// this package but consumed by the OAuth handler (be-auth-oauth) at the wave-4
// composition root — neither package imports the other's concrete types.
type Provisioner struct {
	accounts authdomain.AccountRepo
	oauthIds authdomain.OAuthIdentityRepo
	audit    auditlog.Writer
}

// NewProvisioner creates a Provisioner.
func NewProvisioner(
	accounts authdomain.AccountRepo,
	oauthIds authdomain.OAuthIdentityRepo,
	audit auditlog.Writer,
) *Provisioner {
	return &Provisioner{accounts: accounts, oauthIds: oauthIds, audit: audit}
}

// ProvisionFromOAuth implements authdomain.AccountProvisioner (contract §5).
//
//   - If a linked identity for (provider, subject) already exists: return the
//     associated account.
//   - If a local account owns the email but has no identity row for this
//     provider: return ErrLinkRequired (contract §4.3).
//   - Otherwise: auto-create a new User account with a verified email, link the
//     identity, and emit auth.register.succeeded.
//
// Role is always User (FR-04). The email is treated as verified because Google
// guarantees ownership of the provided email claim (FR-05).
func (p *Provisioner) ProvisionFromOAuth(
	ctx context.Context,
	provider auditmodel.LinkedProvider,
	subject, email string,
) (*authdomain.Account, error) {
	// 1. Already linked?
	acc, err := p.oauthIds.BySubject(ctx, provider, subject)
	if err == nil {
		return acc, nil
	}
	if !errors.Is(err, authdomain.ErrNotFound) {
		return nil, err
	}

	// 2. Does a local account own this email without an identity row?
	emailLower := strings.ToLower(email)
	existing, err := p.accounts.ByEmail(ctx, emailLower)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		// Local account present, no identity → proof-of-control required (FR-06, §4.3).
		return nil, authdomain.ErrLinkRequired
	}

	// 3. Auto-create a new User account. OAuth email claim is treated as verified.
	now := time.Now()
	newAcc := authdomain.Account{
		ID:              uuid.New(),
		Email:           email,
		EmailLower:      emailLower,
		PasswordHash:    "", // OAuth-only; no local password
		Role:            auditmodel.RoleUser,
		Status:          auditmodel.AccountStatusActive,
		EmailVerifiedAt: &now,
		CreatedAt:       now,
	}
	if err := p.accounts.Create(ctx, newAcc); err != nil {
		if errors.Is(err, authdomain.ErrEmailTaken) {
			// Race: another path registered this email just now → treat as link required.
			return nil, authdomain.ErrLinkRequired
		}
		return nil, err
	}

	// 4. Write the identity row.
	if err := p.oauthIds.Link(ctx, newAcc.ID, provider, subject, email); err != nil {
		return nil, err
	}

	// 5. Emit auth.register.succeeded (via=provider so the audit log records
	// this was an OAuth-initiated registration).
	uid := newAcc.ID
	_ = p.audit.Write(ctx, auditmodel.EvtAuthRegisterSucceeded,
		map[string]any{
			"user_id": uid.String(),
			"via":     string(provider),
		},
		auditlog.WriteContext{},
	)

	return &newAcc, nil
}
