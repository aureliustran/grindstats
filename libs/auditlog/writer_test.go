package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"grindstats/libs/auditmodel"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// Validate — required fields
// ---------------------------------------------------------------------------

func TestValidate_MissingRequiredField(t *testing.T) {
	// auth.login.failed requires "reason" and "source_ip"
	err := validate(auditmodel.EvtAuthLoginFailed, map[string]any{
		"source_ip": "1.2.3.4",
		// "reason" deliberately absent
	})
	if err == nil {
		t.Fatal("expected error for missing required field, got nil")
	}
	if !strings.Contains(err.Error(), "reason") {
		t.Errorf("expected error to name missing field 'reason', got: %s", err)
	}
}

func TestValidate_AllRequiredFieldsPresent(t *testing.T) {
	err := validate(auditmodel.EvtAuthLoginFailed, map[string]any{
		"reason":    auditmodel.LoginFailureReasonBadPassword,
		"source_ip": "1.2.3.4",
	})
	if err != nil {
		t.Fatalf("expected no error when all required fields are present, got: %s", err)
	}
}

// ---------------------------------------------------------------------------
// Validate — undeclared fields
// ---------------------------------------------------------------------------

func TestValidate_UndeclaredField(t *testing.T) {
	// auth.logout declares only user_id and session_jti; "extra" is undeclared.
	err := validate(auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     "abc",
		"session_jti": "xyz",
		"extra":       "surprise",
	})
	if err == nil {
		t.Fatal("expected error for undeclared field, got nil")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Errorf("expected error to name undeclared field 'extra', got: %s", err)
	}
}

func TestValidate_DeclaredOptionalField_Accepted(t *testing.T) {
	// auth.login.failed declares optional field "user_id"; it must be accepted.
	err := validate(auditmodel.EvtAuthLoginFailed, map[string]any{
		"reason":    auditmodel.LoginFailureReasonUnknownEmail,
		"source_ip": "1.2.3.4",
		"user_id":   "some-id", // optional, declared
	})
	if err != nil {
		t.Fatalf("expected no error for a declared optional field, got: %s", err)
	}
}

// ---------------------------------------------------------------------------
// Validate — credential-named fields
// ---------------------------------------------------------------------------

func TestValidate_CredentialFieldName_Rejected(t *testing.T) {
	cases := []string{
		"password", "Password", "PASSWORD",
		"token", "secret", "hash", "key", "authorization",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			err := validate(auditmodel.EvtAuthLogout, map[string]any{
				"user_id":     "abc",
				"session_jti": "xyz",
				name:          "value",
			})
			if err == nil {
				t.Fatalf("expected credential field %q to be rejected, got nil", name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validate — unknown event
// ---------------------------------------------------------------------------

func TestValidate_UnknownEvent(t *testing.T) {
	err := validate(auditmodel.AuditEvent("no.such.event"), map[string]any{})
	if err == nil {
		t.Fatal("expected error for unknown event, got nil")
	}
}

// ---------------------------------------------------------------------------
// renderMessage — en-US template substitution
// ---------------------------------------------------------------------------

func TestRenderMessage_BasicSubstitution(t *testing.T) {
	spec := auditmodel.AuditEvents[auditmodel.EvtAuthLoginFailed]
	msg := renderMessage(spec.Message, map[string]any{
		"reason":    auditmodel.LoginFailureReasonBadPassword,
		"source_ip": "192.0.2.1",
	})
	if !strings.Contains(msg, "bad_password") {
		t.Errorf("expected message to contain reason value, got: %q", msg)
	}
	if !strings.Contains(msg, "192.0.2.1") {
		t.Errorf("expected message to contain source_ip, got: %q", msg)
	}
}

func TestRenderMessage_OptionalFieldAbsent_NoLiteralPlaceholder(t *testing.T) {
	// auth.login.succeeded's template contains {device_label} which is optional.
	spec := auditmodel.AuditEvents[auditmodel.EvtAuthLoginSucceeded]
	msg := renderMessage(spec.Message, map[string]any{
		"user_id":     "usr-1",
		"session_jti": "jti-1",
		"source_ip":   "10.0.0.1",
		// device_label deliberately absent
	})
	if strings.Contains(msg, "{device_label}") {
		t.Errorf("literal placeholder must not appear in rendered message, got: %q", msg)
	}
}

// TestRenderMessage_IsEnUsRegardlessOfAmbientState documents that the rendered
// message comes from the event spec template (always en-US) and is not
// influenced by any ambient locale state. There is no locale parameter in the
// Writer API — this test confirms the structural guarantee.
func TestRenderMessage_IsEnUsRegardlessOfAmbientState(t *testing.T) {
	spec := auditmodel.AuditEvents[auditmodel.EvtAuthLogout]
	want := "Logout for usr-42"

	// Call renderMessage with no locale involved — there is no way to pass one.
	got := renderMessage(spec.Message, map[string]any{"user_id": "usr-42", "session_jti": "jti"})
	if got != want {
		t.Errorf("renderMessage = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// compactEnumValue
// ---------------------------------------------------------------------------

func TestCompactEnumValue_LoginFailureReason(t *testing.T) {
	got := compactEnumValue(auditmodel.LoginFailureReasonBadPassword)
	want := auditmodel.LoginFailureReasonCompact[auditmodel.LoginFailureReasonBadPassword]
	if got != want {
		t.Errorf("compactEnumValue = %v, want %v", got, want)
	}
}

func TestCompactEnumValue_TokenRejectReason(t *testing.T) {
	got := compactEnumValue(auditmodel.TokenRejectReasonExpired)
	want := auditmodel.TokenRejectReasonCompact[auditmodel.TokenRejectReasonExpired]
	if got != want {
		t.Errorf("compactEnumValue = %v, want %v", got, want)
	}
}

func TestCompactEnumValue_NonEnum_PassedThrough(t *testing.T) {
	cases := []any{"some-string", 42, true, uuid.MustParse("00000000-0000-0000-0000-000000000001")}
	for _, v := range cases {
		got := compactEnumValue(v)
		if got != v {
			t.Errorf("compactEnumValue(%v) = %v, want identity", v, got)
		}
	}
}

func TestCompactEnumValue_AllEnumsHaveCompactCode(t *testing.T) {
	// Sanity-check: every enum value we might receive should have a compact code.
	for _, v := range auditmodel.AllLoginFailureReasons {
		c := compactEnumValue(v)
		if fmt.Sprint(c) == string(v) {
			t.Errorf("LoginFailureReason %q was not compacted", v)
		}
	}
	for _, v := range auditmodel.AllTokenRejectReasons {
		c := compactEnumValue(v)
		if fmt.Sprint(c) == string(v) {
			t.Errorf("TokenRejectReason %q was not compacted", v)
		}
	}
	for _, v := range auditmodel.AllAdminActions {
		c := compactEnumValue(v)
		if fmt.Sprint(c) == string(v) {
			t.Errorf("AdminAction %q was not compacted", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Compact code round-trip (unit layer of the integration test)
// ---------------------------------------------------------------------------

func TestWriter_CompactCodes_UnitPreview(t *testing.T) {
	// Build what the writer would store, without a real DB.
	event := auditmodel.EvtAuthRefreshReplayDetected
	fields := map[string]any{
		"user_id":      "usr-1",
		"replayed_jti": "old-jti",
		"source_ip":    "10.0.0.1",
	}

	// Validate first (same as the writer does).
	if err := validate(event, fields); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	spec := auditmodel.AuditEvents[event]

	// Column codes should all be 8 chars.
	eventCode := auditmodel.AuditEventCompact[event]
	if len(eventCode) != 8 {
		t.Errorf("event compact code %q is not 8 chars", eventCode)
	}
	actorCode := auditmodel.ActorTypeCompact[spec.Actor]
	if len(actorCode) != 8 {
		t.Errorf("actor_type compact code %q is not 8 chars", actorCode)
	}
	outcomeCode := auditmodel.OutcomeCompact[spec.Outcome]
	if len(outcomeCode) != 8 {
		t.Errorf("outcome compact code %q is not 8 chars", outcomeCode)
	}
	severityCode := auditmodel.SeverityCompact[spec.Severity]
	if len(severityCode) != 8 {
		t.Errorf("severity compact code %q is not 8 chars", severityCode)
	}

	// The compacted fields map should round-trip through JSON.
	compacted := make(map[string]any, len(fields))
	for k, v := range fields {
		compacted[k] = compactEnumValue(v)
	}
	raw, err := json.Marshal(compacted)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var check map[string]any
	if err := json.Unmarshal(raw, &check); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Rendered message must be en-US and contain no literal placeholders.
	msg := renderMessage(spec.Message, fields)
	if strings.Contains(msg, "{") {
		t.Errorf("rendered message contains unresolved placeholder: %q", msg)
	}
	if !strings.Contains(msg, "usr-1") {
		t.Errorf("rendered message missing user_id: %q", msg)
	}
}

// ---------------------------------------------------------------------------
// Integration test — database-gated (GRINDSTATS_TEST_DB)
// ---------------------------------------------------------------------------

// TestPgxWriter_Integration_ValidEventRoundTrips verifies that a valid event
// round-trips through the writer: readable in → compact stored → en-US message
// rendered. Requires a running Postgres with the audit schema.
//
// Run with:
//
//	GRINDSTATS_TEST_DB="postgres://..." go test ./libs/auditlog/...
func TestPgxWriter_Integration_ValidEventRoundTrips(t *testing.T) {
	dsn := os.Getenv("GRINDSTATS_TEST_DB")
	if dsn == "" {
		t.Skip("GRINDSTATS_TEST_DB not set; skipping database-gated integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	w := New(pool, nil)

	actorID := ptr(uuid.New())
	wctx := WriteContext{
		ActorUserID: actorID,
		RequestID:   "req-integration-test",
	}

	event := auditmodel.EvtAuthLoginFailed
	fields := map[string]any{
		"reason":    auditmodel.LoginFailureReasonBadPassword,
		"source_ip": "198.51.100.1",
	}

	if err := w.Write(ctx, event, fields, wctx); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Read the row back and verify compact codes were stored.
	var (
		storedEventCode    string
		storedActorType    string
		storedOutcome      string
		storedSeverity     string
		storedFieldsRaw    []byte
		storedMessage      string
		storedActorUserID  uuid.UUID
	)
	row := pool.QueryRow(ctx, `
		SELECT event_code, actor_type, outcome, severity, fields, message, actor_user_id
		FROM audit.records
		WHERE request_id = $1
		ORDER BY occurred_at DESC
		LIMIT 1`, wctx.RequestID)
	if err := row.Scan(
		&storedEventCode, &storedActorType, &storedOutcome, &storedSeverity,
		&storedFieldsRaw, &storedMessage, &storedActorUserID,
	); err != nil {
		t.Fatalf("scan row: %v", err)
	}

	wantEventCode := auditmodel.AuditEventCompact[event]
	if storedEventCode != wantEventCode {
		t.Errorf("event_code = %q, want %q", storedEventCode, wantEventCode)
	}

	spec := auditmodel.AuditEvents[event]
	if storedActorType != auditmodel.ActorTypeCompact[spec.Actor] {
		t.Errorf("actor_type = %q, want %q", storedActorType, auditmodel.ActorTypeCompact[spec.Actor])
	}
	if storedOutcome != auditmodel.OutcomeCompact[spec.Outcome] {
		t.Errorf("outcome = %q, want %q", storedOutcome, auditmodel.OutcomeCompact[spec.Outcome])
	}
	if storedSeverity != auditmodel.SeverityCompact[spec.Severity] {
		t.Errorf("severity = %q, want %q", storedSeverity, auditmodel.SeverityCompact[spec.Severity])
	}

	// Verify the "reason" field was stored as its compact code.
	var storedFields map[string]any
	if err := json.Unmarshal(storedFieldsRaw, &storedFields); err != nil {
		t.Fatalf("unmarshal stored fields: %v", err)
	}
	wantReasonCode := auditmodel.LoginFailureReasonCompact[auditmodel.LoginFailureReasonBadPassword]
	if storedFields["reason"] != wantReasonCode {
		t.Errorf("fields[reason] = %v, want %q (compact code)", storedFields["reason"], wantReasonCode)
	}

	// Message must be en-US and contain the human-readable reason.
	if !strings.Contains(storedMessage, "bad_password") {
		t.Errorf("message %q should contain readable reason 'bad_password'", storedMessage)
	}

	// actor_user_id must round-trip.
	if storedActorUserID != *actorID {
		t.Errorf("actor_user_id = %v, want %v", storedActorUserID, *actorID)
	}
}
