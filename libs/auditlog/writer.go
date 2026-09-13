// Package auditlog is the one way an audit record is written.
//
// Every audit write goes through this package. docs/audit-and-errors.md §3
// defines the event shape; §6 defines the compact-code storage format this
// package implements at the write boundary.
//
// # Three language planes — do not conflate
//
// Audit records are ALWAYS written in en-US (docs/audit-and-errors.md §1a).
// The Write signature has no locale parameter; making it structurally
// impossible to pass a locale is stronger than documenting that it would be
// wrong.
//
// # Error contract
//
// Validation failures (missing required field, undeclared field,
// credential-named field, unknown event) are returned as errors — they are
// programming bugs that should surface during development, not production
// surprises.
//
// Database-write failures are logged at error level and swallowed; a missing
// audit row is better than a 500 reaching the user whose action triggered it.
// The Fake surfaces these would-be errors so handler tests can assert them.
package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/libs/auditmodel"
)

// credentialFieldNames is the runtime belt to the generator's build-time
// braces (docs/audit-and-errors.md §3, NFR-07). A field whose name looks like
// a credential is rejected regardless of its value — the generator performs
// this check at model-compile time, but a field value assembled at runtime is
// exactly what the generator cannot see.
var credentialFieldNames = map[string]bool{
	"password":      true,
	"token":         true,
	"secret":        true,
	"hash":          true,
	"key":           true,
	"authorization": true,
}

// placeholderRe matches the {field} placeholders used in AuditEventSpec.Message.
var placeholderRe = regexp.MustCompile(`\{([^}]+)\}`)

// WriteContext carries the per-request identifiers for an audit record. There
// is deliberately no locale field — see package doc.
type WriteContext struct {
	// ActorUserID is the authenticated user who triggered the action.
	// Nil for anonymous actors (registration, login attempts).
	ActorUserID *uuid.UUID
	// TargetUserID is the user the action was performed on (e.g. the account
	// an admin suspended). Nil when there is no distinct target.
	TargetUserID *uuid.UUID
	// RequestID links the audit record to the operational log entry for the
	// same request (request-ID middleware, GATE-001).
	RequestID string
}

// Writer is the single way an audit record is persisted. Callers pass readable
// constants (auditmodel.EvtAuthLoginFailed, auditmodel.LoginFailureReasonBadPassword)
// and never deal with compact codes — translation to the 8-char DB format
// happens inside Write.
//
// See the package doc for the full error contract.
type Writer interface {
	Write(ctx context.Context, event auditmodel.AuditEvent, fields map[string]any, wctx WriteContext) error
}

// PgxWriter inserts audit records into the audit.records table (contract §4)
// via a pgx connection pool. There is no update or delete path — audit records
// are append-only (FR-45).
type PgxWriter struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// New returns a PgxWriter backed by pool. If logger is nil, slog.Default() is
// used for error-level DB-write failures.
func New(pool *pgxpool.Pool, logger *slog.Logger) *PgxWriter {
	if logger == nil {
		logger = slog.Default()
	}
	return &PgxWriter{pool: pool, logger: logger}
}

// Write validates the call, renders the en-US message, translates all codes to
// their compact 8-char form, and inserts one row into audit.records. A
// validation error is returned to the caller (programming bug). A DB error is
// logged at error level and swallowed (Write always returns nil for DB errors).
//
// There is no update path and no delete path. A correction is a new record.
func (w *PgxWriter) Write(
	ctx context.Context,
	event auditmodel.AuditEvent,
	fields map[string]any,
	wctx WriteContext,
) error {
	if err := validate(event, fields); err != nil {
		return err
	}

	spec := auditmodel.AuditEvents[event]

	// Render en-US message. No locale is involved; the message template in the
	// spec is already the en-US string, and there is no parameter through which
	// a different locale could reach this point.
	msg := renderMessage(spec.Message, fields)

	// Translate to compact 8-char codes at the storage boundary.
	// These compact codes never appear in the API we expose — callers pass
	// readable constants; compact codes exist only in the DB row.
	eventCode := auditmodel.AuditEventCompact[event]
	actorCode := auditmodel.ActorTypeCompact[spec.Actor]
	outcomeCode := auditmodel.OutcomeCompact[spec.Outcome]
	severityCode := auditmodel.SeverityCompact[spec.Severity]

	// Compact enum-valued fields before storing in JSONB.
	compacted := make(map[string]any, len(fields))
	for k, v := range fields {
		compacted[k] = compactEnumValue(v)
	}
	fieldsJSON, err := json.Marshal(compacted)
	if err != nil {
		w.logger.ErrorContext(ctx, "auditlog: marshal fields",
			"error", err, "event", string(event))
		return nil // swallowed — see package doc
	}

	const insertSQL = `
INSERT INTO audit.records
  (occurred_at, event_code, actor_type, outcome, severity,
   actor_user_id, target_user_id, request_id, fields, message)
VALUES (now(), $1, $2, $3, $4, $5, $6, $7, $8, $9)`

	if _, err = w.pool.Exec(ctx, insertSQL,
		eventCode, actorCode, outcomeCode, severityCode,
		wctx.ActorUserID, wctx.TargetUserID,
		nilIfEmpty(wctx.RequestID),
		json.RawMessage(fieldsJSON), msg,
	); err != nil {
		w.logger.ErrorContext(ctx, "auditlog: insert audit record",
			"error", err,
			"event", string(event),
			"request_id", wctx.RequestID,
		)
		// Swallowed. A missing audit row is preferable to a 500 reaching the
		// user whose action triggered the write (brief §contract, FR-45 note).
	}
	return nil
}

// validate checks that every required field is present, no credential-named
// fields are included, and — for known events — no undeclared field slips in.
// These are all programming errors that should surface at development time.
func validate(event auditmodel.AuditEvent, fields map[string]any) error {
	// Credential-field check first: reject before storing anything.
	for k := range fields {
		if credentialFieldNames[strings.ToLower(k)] {
			return fmt.Errorf(
				"auditlog: field %q looks like a credential and must never appear "+
					"in an audit record (NFR-07; see docs/audit-and-errors.md §3)",
				k,
			)
		}
	}

	spec, ok := auditmodel.AuditEvents[event]
	if !ok {
		return fmt.Errorf("auditlog: unknown audit event %q", event)
	}

	// Required-field check: every declared-required field must be supplied.
	for _, req := range spec.RequiredFields {
		if _, present := fields[req]; !present {
			return fmt.Errorf("auditlog: event %q requires field %q", event, req)
		}
	}

	// Undeclared-field check: the full declared set is RequiredFields plus
	// OptionalFields (AMD-002, docs/stories/auth-epic/amendments/
	// AMD-002-audit-event-spec-all-fields.md) — a field present in neither is
	// a typo, a copy-paste error, or a field from the wrong event.
	declaredSet := make(map[string]bool, len(spec.RequiredFields)+len(spec.OptionalFields))
	for _, f := range spec.RequiredFields {
		declaredSet[f] = true
	}
	for _, f := range spec.OptionalFields {
		declaredSet[f] = true
	}
	for k := range fields {
		if !declaredSet[k] {
			return fmt.Errorf(
				"auditlog: event %q does not declare field %q", event, k,
			)
		}
	}

	return nil
}

// renderMessage substitutes {field} placeholders in the en-US message template
// with the corresponding values from fields. Optional fields absent from fields
// produce an empty substitution; this is acceptable because the structured
// fields column is the queryable record — the message is a human-readable
// convenience for reading a single row.
func renderMessage(tmpl string, fields map[string]any) string {
	return placeholderRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := match[1 : len(match)-1] // strip leading { and trailing }
		if val, ok := fields[key]; ok {
			return fmt.Sprint(val)
		}
		return "" // absent optional field
	})
}

// compactEnumValue translates a Go enum constant to its generated 8-char
// compact DB code (docs/audit-and-errors.md §6). Non-enum values (strings,
// ints, UUIDs, booleans) are returned unchanged.
//
// Callers pass typed constants (auditmodel.LoginFailureReasonBadPassword,
// auditmodel.AdminActionSuspend, …) not raw strings; the type switch is what
// makes this safe without a combined lookup table (which would conflict on
// values like "active" appearing in multiple enums with different prefixes).
//
func compactEnumValue(v any) any {
	switch tv := v.(type) {
	case auditmodel.LoginFailureReason:
		if c, ok := auditmodel.LoginFailureReasonCompact[tv]; ok {
			return c
		}
	case auditmodel.TokenRejectReason:
		if c, ok := auditmodel.TokenRejectReasonCompact[tv]; ok {
			return c
		}
	case auditmodel.AdminAction:
		if c, ok := auditmodel.AdminActionCompact[tv]; ok {
			return c
		}
	case auditmodel.LinkedProvider:
		if c, ok := auditmodel.LinkedProviderCompact[tv]; ok {
			return c
		}
	case auditmodel.AccountStatus:
		if c, ok := auditmodel.AccountStatusCompact[tv]; ok {
			return c
		}
	case auditmodel.LinkKind:
		if c, ok := auditmodel.LinkKindCompact[tv]; ok {
			return c
		}
	case auditmodel.LinkRejectReason:
		if c, ok := auditmodel.LinkRejectReasonCompact[tv]; ok {
			return c
		}
	case auditmodel.Role:
		if c, ok := auditmodel.RoleCompact[tv]; ok {
			return c
		}
	case auditmodel.ActorType:
		if c, ok := auditmodel.ActorTypeCompact[tv]; ok {
			return c
		}
	case auditmodel.Outcome:
		if c, ok := auditmodel.OutcomeCompact[tv]; ok {
			return c
		}
	case auditmodel.Severity:
		if c, ok := auditmodel.SeverityCompact[tv]; ok {
			return c
		}
	}
	return v
}

// nilIfEmpty returns nil for an empty string, making request_id NULL in the DB
// rather than an empty-string row that queries must work around.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
