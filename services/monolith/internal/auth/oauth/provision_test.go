package oauth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"grindstats/libs/auditlog"
	"grindstats/libs/auditmodel"
	"grindstats/services/monolith/internal/auth/authdomain"
	"grindstats/services/monolith/internal/auth/credentials"
)

// Scenario: first Google login auto-creates an account (TC-07)
func TestProvision_first_Google_login_auto_creates_an_account(t *testing.T) {
	accounts := newFakeAccountRepo()
	oauthIds := newFakeOAuthIdentityRepo()
	fake := auditlog.NewFake()

	provisioner := credentials.NewProvisioner(accounts, oauthIds, fake)

	acc, err := provisioner.ProvisionFromOAuth(
		context.Background(),
		auditmodel.LinkedProviderGoogle,
		"google-sub-123",
		"newuser@example.com",
	)

	require.NoError(t, err)
	require.NotNil(t, acc)

	// Account must be role User (FR-04), never system_admin
	assert.Equal(t, auditmodel.RoleUser, acc.Role)

	// Email must be treated as verified (Google guarantees ownership)
	assert.NotNil(t, acc.EmailVerifiedAt,
		"OAuth-created account must have a verified email")

	// Password hash must be empty (OAuth-only account)
	assert.Empty(t, acc.PasswordHash,
		"OAuth-only account must not have a password hash")

	// Email must be stored as provided
	assert.Equal(t, "newuser@example.com", acc.Email)
	assert.Equal(t, "newuser@example.com", acc.EmailLower)

	// Account must have been persisted
	persisted, err := accounts.ByEmail(context.Background(), "newuser@example.com")
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, acc.ID, persisted.ID)

	// OAuth identity row must have been linked
	require.Len(t, oauthIds.calls, 1)
	assert.Equal(t, acc.ID, oauthIds.calls[0].userID)
	assert.Equal(t, auditmodel.LinkedProviderGoogle, oauthIds.calls[0].provider)
	assert.Equal(t, "google-sub-123", oauthIds.calls[0].subject)

	// auth.register.succeeded must be emitted
	evts := fake.EventsOf(auditmodel.EvtAuthRegisterSucceeded)
	require.Len(t, evts, 1)
	assert.Equal(t, acc.ID.String(), evts[0].Fields["user_id"])
	assert.Equal(t, string(auditmodel.LinkedProviderGoogle), evts[0].Fields["via"])
}

// ProvisionFromOAuth with an existing linked account returns the account.
func TestProvision_existing_linked_identity_returns_account(t *testing.T) {
	accounts := newFakeAccountRepo()
	oauthIds := newFakeOAuthIdentityRepo()
	fake := auditlog.NewFake()

	existingID := uuid.New()
	at := time.Now()
	existingAcc := authdomain.Account{
		ID:              existingID,
		Email:           "linked@example.com",
		EmailLower:      "linked@example.com",
		Role:            auditmodel.RoleUser,
		Status:          auditmodel.AccountStatusActive,
		EmailVerifiedAt: &at,
		CreatedAt:       time.Now(),
	}
	accounts.seed(existingAcc)
	oauthIds.seedLinked(auditmodel.LinkedProviderGoogle, "sub-existing", existingAcc)

	provisioner := credentials.NewProvisioner(accounts, oauthIds, fake)

	acc, err := provisioner.ProvisionFromOAuth(
		context.Background(),
		auditmodel.LinkedProviderGoogle,
		"sub-existing",
		"linked@example.com",
	)

	require.NoError(t, err)
	require.NotNil(t, acc)
	assert.Equal(t, existingID, acc.ID)

	// No new account created, no new identity linked, no audit event for register
	assert.Empty(t, accounts.created)
	assert.Empty(t, oauthIds.calls)
	assert.Len(t, fake.EventsOf(auditmodel.EvtAuthRegisterSucceeded), 0)
}

// ProvisionFromOAuth with a local account owning the email returns ErrLinkRequired.
func TestProvision_local_account_without_identity_returns_ErrLinkRequired(t *testing.T) {
	accounts := newFakeAccountRepo()
	oauthIds := newFakeOAuthIdentityRepo()

	// Seed a local account with the email, but no oauth_identities row
	accounts.seed(authdomain.Account{
		ID:         uuid.New(),
		Email:      "local@example.com",
		EmailLower: "local@example.com",
		Role:       auditmodel.RoleUser,
		Status:     auditmodel.AccountStatusActive,
		CreatedAt:  time.Now(),
	})

	provisioner := credentials.NewProvisioner(accounts, oauthIds, auditlog.NewFake())

	_, err := provisioner.ProvisionFromOAuth(
		context.Background(),
		auditmodel.LinkedProviderGoogle,
		"google-sub-xyz",
		"local@example.com",
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, authdomain.ErrLinkRequired,
		"must return ErrLinkRequired when a local account owns the email")
}
