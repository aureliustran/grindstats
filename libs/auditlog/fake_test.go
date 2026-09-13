package auditlog

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"grindstats/libs/auditmodel"
)

// ---------------------------------------------------------------------------
// Fake — records what the real writer would have written
// ---------------------------------------------------------------------------

func TestFake_RecordsEvent(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	actorID := ptr(uuid.New())
	err := f.Write(ctx, auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     "usr-1",
		"session_jti": "jti-1",
	}, WriteContext{ActorUserID: actorID, RequestID: "req-1"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if f.Count() != 1 {
		t.Fatalf("Count = %d, want 1", f.Count())
	}
	recs := f.Records()
	if len(recs) != 1 {
		t.Fatalf("len(Records) = %d, want 1", len(recs))
	}
	if recs[0].Event != auditmodel.EvtAuthLogout {
		t.Errorf("Event = %q, want %q", recs[0].Event, auditmodel.EvtAuthLogout)
	}
	if recs[0].ActorUserID != actorID {
		t.Errorf("ActorUserID = %v, want %v", recs[0].ActorUserID, actorID)
	}
	if recs[0].RequestID != "req-1" {
		t.Errorf("RequestID = %q, want %q", recs[0].RequestID, "req-1")
	}
}

// TestFake_SeverityAndOutcomeFromSpec verifies that the Fake correctly
// propagates the event spec's actor type, outcome and severity — the Fake
// would be useless for assertions if it invented its own.
func TestFake_SeverityAndOutcomeFromSpec(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	uid := ptr(uuid.New())
	_ = f.Write(ctx, auditmodel.EvtAuthRefreshReplayDetected, map[string]any{
		"user_id":      uid.String(),
		"replayed_jti": "old-jti",
		"source_ip":    "10.0.0.1",
	}, WriteContext{ActorUserID: uid})

	recs := f.EventsOf(auditmodel.EvtAuthRefreshReplayDetected)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Severity != auditmodel.SeverityCritical {
		t.Errorf("Severity = %q, want %q", recs[0].Severity, auditmodel.SeverityCritical)
	}
	if recs[0].Outcome != auditmodel.OutcomeDenied {
		t.Errorf("Outcome = %q, want %q", recs[0].Outcome, auditmodel.OutcomeDenied)
	}
	if recs[0].ActorType != auditmodel.ActorTypeUser {
		t.Errorf("ActorType = %q, want %q", recs[0].ActorType, auditmodel.ActorTypeUser)
	}
}

// TestFake_FieldAssertionByValue exercises the primary use case: a handler test
// asserting "exactly one auth.refresh.replay_detected with severity critical
// and replayed_jti equal to X". This is the pattern all wave-3 slices rely on.
func TestFake_FieldAssertionByValue(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	uid := ptr(uuid.New())
	wantJTI := "the-replayed-jti"

	_ = f.Write(ctx, auditmodel.EvtAuthRefreshReplayDetected, map[string]any{
		"user_id":      uid.String(),
		"replayed_jti": wantJTI,
		"source_ip":    "10.0.0.2",
	}, WriteContext{ActorUserID: uid})

	events := f.EventsOf(auditmodel.EvtAuthRefreshReplayDetected)
	if len(events) != 1 {
		t.Fatalf("expected exactly one replay-detected event, got %d", len(events))
	}
	if events[0].Severity != auditmodel.SeverityCritical {
		t.Errorf("Severity = %q, want critical", events[0].Severity)
	}
	if events[0].Fields["replayed_jti"] != wantJTI {
		t.Errorf("Fields[replayed_jti] = %v, want %q", events[0].Fields["replayed_jti"], wantJTI)
	}
}

// TestFake_FieldsAreReadable confirms fields are stored as the caller passed
// them (readable constants), not as compact codes. Wave-3 handler tests assert
// against auditmodel constants, not 8-char DB codes.
func TestFake_FieldsAreReadable(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	_ = f.Write(ctx, auditmodel.EvtAuthLoginFailed, map[string]any{
		"reason":    auditmodel.LoginFailureReasonUnknownEmail,
		"source_ip": "10.0.0.3",
	}, WriteContext{})

	recs := f.EventsOf(auditmodel.EvtAuthLoginFailed)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	// Must be the readable constant, not the compact code.
	if recs[0].Fields["reason"] != auditmodel.LoginFailureReasonUnknownEmail {
		t.Errorf("Fields[reason] = %v, want %q (readable constant, not compact code)",
			recs[0].Fields["reason"], auditmodel.LoginFailureReasonUnknownEmail)
	}
}

// TestFake_ValidationErrors confirms the Fake surfaces validation errors rather
// than swallowing them — this is what makes it useful for catching bugs in test.
func TestFake_ValidationErrors_Surfaced(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	// Missing required field
	err := f.Write(ctx, auditmodel.EvtAuthLoginFailed, map[string]any{
		"source_ip": "10.0.0.4",
		// "reason" missing
	}, WriteContext{})
	if err == nil {
		t.Error("expected error for missing required field, got nil")
	}
	if f.Count() != 0 {
		t.Errorf("Count = %d, expected 0 after failed write", f.Count())
	}

	// Credential-named field
	err = f.Write(ctx, auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     "usr",
		"session_jti": "jti",
		"password":    "whoops",
	}, WriteContext{})
	if err == nil {
		t.Error("expected error for credential-named field, got nil")
	}
	if f.Count() != 0 {
		t.Errorf("Count = %d, expected 0 after credential-field error", f.Count())
	}
}

// TestFake_EventsOf_FiltersByEvent confirms EventsOf only returns records for
// the requested event type.
func TestFake_EventsOf_FiltersByEvent(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	uid := ptr(uuid.New())

	_ = f.Write(ctx, auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     uid.String(),
		"session_jti": "j1",
	}, WriteContext{ActorUserID: uid})
	_ = f.Write(ctx, auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     uid.String(),
		"session_jti": "j2",
	}, WriteContext{ActorUserID: uid})
	_ = f.Write(ctx, auditmodel.EvtAuthLogoutAll, map[string]any{
		"user_id": uid.String(),
	}, WriteContext{ActorUserID: uid})

	logouts := f.EventsOf(auditmodel.EvtAuthLogout)
	if len(logouts) != 2 {
		t.Errorf("EventsOf(EvtAuthLogout) = %d records, want 2", len(logouts))
	}
	logoutAlls := f.EventsOf(auditmodel.EvtAuthLogoutAll)
	if len(logoutAlls) != 1 {
		t.Errorf("EventsOf(EvtAuthLogoutAll) = %d records, want 1", len(logoutAlls))
	}
}

// TestFake_Reset clears all records.
func TestFake_Reset(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	uid := ptr(uuid.New())

	_ = f.Write(ctx, auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     uid.String(),
		"session_jti": "j1",
	}, WriteContext{ActorUserID: uid})

	if f.Count() != 1 {
		t.Fatalf("Count = %d before reset, want 1", f.Count())
	}
	f.Reset()
	if f.Count() != 0 {
		t.Errorf("Count = %d after Reset, want 0", f.Count())
	}
	if len(f.Records()) != 0 {
		t.Errorf("Records() = %d after Reset, want 0", len(f.Records()))
	}
}

// TestFake_MessageRenderedEnUs confirms the message is rendered from the en-US
// template, not from any locale parameter (there is none).
func TestFake_MessageRenderedEnUs(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	uid := ptr(uuid.New())

	_ = f.Write(ctx, auditmodel.EvtAuthLogout, map[string]any{
		"user_id":     uid.String(),
		"session_jti": "j1",
	}, WriteContext{ActorUserID: uid})

	recs := f.EventsOf(auditmodel.EvtAuthLogout)
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	// The en-US template is "Logout for {user_id}".
	want := "Logout for " + uid.String()
	if recs[0].Message != want {
		t.Errorf("Message = %q, want %q", recs[0].Message, want)
	}
}

// TestFake_ImplementsWriter confirms at compile time that *Fake satisfies the
// Writer interface. If this fails, the wave-3 slices cannot inject a Fake.
func TestFake_ImplementsWriter(t *testing.T) {
	var _ Writer = (*Fake)(nil)
}
