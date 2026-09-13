package auditlog

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"grindstats/libs/auditmodel"
)

// Record is one audit entry as the Fake recorded it — fields in their readable
// (pre-compact) form, exactly as the caller passed them. Handler tests assert
// against this struct rather than against DB rows.
type Record struct {
	// Event is the audit event constant (e.g. auditmodel.EvtAuthRefreshReplayDetected).
	Event auditmodel.AuditEvent
	// ActorType, Outcome, Severity come from the event spec — they are set by
	// the model, not by the caller.
	ActorType auditmodel.ActorType
	Outcome   auditmodel.Outcome
	Severity  auditmodel.Severity
	// Fields are the caller-supplied field map, validated but NOT compacted.
	// Tests assert against readable constants:
	//   rec.Fields["replayed_jti"] == "some-jti-value"
	Fields map[string]any
	// Message is the rendered en-US message string.
	Message string
	// Per-call context, as passed by the handler.
	ActorUserID  *uuid.UUID
	TargetUserID *uuid.UUID
	RequestID    string
}

// Fake implements Writer in memory. It performs the same validation as the
// real writer (required fields, undeclared fields, credential field names) and
// returns those errors rather than swallowing them — making test failures
// visible immediately rather than silently missing records. There is no database;
// write errors in the real writer that would be swallowed simply don't occur here.
//
// The Fake is safe for concurrent use.
//
// Typical handler-test usage:
//
//	fake := auditlog.NewFake()
//	// inject fake into handler under test
//
//	// exercise the handler
//	handler.ServeHTTP(rec, req)
//
//	// assert: exactly one replay-detected event with the right jti
//	events := fake.EventsOf(auditmodel.EvtAuthRefreshReplayDetected)
//	require.Len(t, events, 1)
//	assert.Equal(t, auditmodel.SeverityCritical, events[0].Severity)
//	assert.Equal(t, "the-replayed-jti", events[0].Fields["replayed_jti"])
type Fake struct {
	mu      sync.Mutex
	records []Record
}

// NewFake returns an empty Fake ready to use.
func NewFake() *Fake {
	return &Fake{}
}

// Write validates the call (same rules as PgxWriter) and records the event
// in memory. Unlike PgxWriter, validation errors are returned rather than
// silently dropped, so test failures surface immediately.
func (f *Fake) Write(
	ctx context.Context,
	event auditmodel.AuditEvent,
	fields map[string]any,
	wctx WriteContext,
) error {
	if err := validate(event, fields); err != nil {
		return err
	}

	spec := auditmodel.AuditEvents[event]
	msg := renderMessage(spec.Message, fields)

	// Store fields as-is (readable, not compacted) so tests can assert against
	// auditmodel constants rather than 8-char storage codes.
	copied := make(map[string]any, len(fields))
	for k, v := range fields {
		copied[k] = v
	}

	rec := Record{
		Event:        event,
		ActorType:    spec.Actor,
		Outcome:      spec.Outcome,
		Severity:     spec.Severity,
		Fields:       copied,
		Message:      msg,
		ActorUserID:  wctx.ActorUserID,
		TargetUserID: wctx.TargetUserID,
		RequestID:    wctx.RequestID,
	}

	f.mu.Lock()
	f.records = append(f.records, rec)
	f.mu.Unlock()

	return nil
}

// Records returns a snapshot of every record written so far, in write order.
func (f *Fake) Records() []Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Record, len(f.records))
	copy(out, f.records)
	return out
}

// EventsOf returns all records for the given event, in write order.
func (f *Fake) EventsOf(event auditmodel.AuditEvent) []Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Record
	for _, r := range f.records {
		if r.Event == event {
			out = append(out, r)
		}
	}
	return out
}

// Count returns the total number of records written so far.
func (f *Fake) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

// Reset discards all recorded events. Use between test cases when reusing a Fake.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = nil
}
