// Package mailer defines the interface for sending the three emailed link types
// (email verification, password reset, OAuth link proof). Production delivery
// infrastructure is out of scope for AUTH-001. The only concrete implementation
// here is DevMailer, which logs the deep-link to the operational log — it is
// opt-in via the AUTH_DEV_MAILER=true env var that the compose file sets.
//
// Never log a raw link token in production mode. DevMailer is the one place a
// token may appear in a log, and it is guarded by the dev opt-in.
package mailer

import (
	"context"
	"log/slog"
)

// Mailer sends the three emailed single-use links the auth flow needs.
type Mailer interface {
	// SendVerification emails the email-verification deep-link to toEmail.
	// rawToken is the unencoded token the user pastes or clicks.
	SendVerification(ctx context.Context, toEmail, rawToken string) error

	// SendPasswordReset emails the password-reset deep-link.
	SendPasswordReset(ctx context.Context, toEmail, rawToken string) error

	// SendOAuthLink emails the OAuth linking-proof deep-link (contract §4.3).
	SendOAuthLink(ctx context.Context, toEmail, rawToken string) error
}

// DevMailer writes links to the structured log instead of sending email.
// It must only be used when AUTH_DEV_MAILER=true; the be-wiring slice controls
// which concrete type is injected. In any environment where the structured log
// is shipped to an external store, this implementation leaks single-use tokens
// — production wiring must substitute a real mailer.
type DevMailer struct {
	logger  *slog.Logger
	baseURL string // e.g. "http://localhost:3000"
}

// NewDev returns a DevMailer. If logger is nil, slog.Default() is used.
func NewDev(logger *slog.Logger, baseURL string) *DevMailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &DevMailer{logger: logger, baseURL: baseURL}
}

// SendVerification logs the verification link.
func (m *DevMailer) SendVerification(ctx context.Context, toEmail, rawToken string) error {
	m.logger.InfoContext(ctx, "DEV MAILER: verification link",
		"to", toEmail,
		"link", m.baseURL+"/verify-email?token="+rawToken,
	)
	return nil
}

// SendPasswordReset logs the password-reset link.
func (m *DevMailer) SendPasswordReset(ctx context.Context, toEmail, rawToken string) error {
	m.logger.InfoContext(ctx, "DEV MAILER: password-reset link",
		"to", toEmail,
		"link", m.baseURL+"/password-reset/confirm?token="+rawToken,
	)
	return nil
}

// SendOAuthLink logs the OAuth linking-proof link (contract §4.3).
func (m *DevMailer) SendOAuthLink(ctx context.Context, toEmail, rawToken string) error {
	m.logger.InfoContext(ctx, "DEV MAILER: oauth link",
		"to", toEmail,
		"link", m.baseURL+"/oauth/link/confirm?token="+rawToken,
	)
	return nil
}
