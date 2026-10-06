package consent

import (
	"context"
	"testing"
)

type fixedChecker bool

func (f fixedChecker) Check(context.Context, string, string) (bool, error) {
	return bool(f), nil
}

func TestAuthorizeOperation(t *testing.T) {
	tests := []struct {
		name    string
		kind    OperationKind
		granted bool
		allowed bool
		reason  string
	}{
		{name: "consented build", kind: BuildEvent, granted: true, allowed: true, reason: "consent_granted"},
		{name: "release without consent", kind: ReleaseOperation, granted: false, allowed: false, reason: "consent_required"},
		{name: "consented diagnostics", kind: DiagnosticAccess, granted: true, allowed: true, reason: "consent_granted"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := Operation{EventID: "evt-17", UserID: "user-42", Kind: test.kind, Category: "developer_tools"}
			got, err := AuthorizeOperation(context.Background(), fixedChecker(test.granted), input)
			if err != nil {
				t.Fatalf("AuthorizeOperation() error = %v", err)
			}
			if got.Allowed != test.allowed || got.Reason != test.reason || got.EventID != input.EventID {
				t.Fatalf("AuthorizeOperation() = %#v", got)
			}
		})
	}
}

type recordingWriter struct {
	action string
	key    string
}

func (w *recordingWriter) Grant(_ context.Context, _, _, _, key string) error {
	w.action, w.key = "grant", key
	return nil
}

func (w *recordingWriter) Revoke(_ context.Context, _, _, key string) error {
	w.action, w.key = "revoke", key
	return nil
}

func TestApplyChangeUsesStableEventIdentity(t *testing.T) {
	writer := &recordingWriter{}
	got, err := ApplyChange(context.Background(), writer, Change{
		EventID: "evt-91", UserID: "user-42", Category: "release_operations", Action: "revoke",
	})
	if err != nil {
		t.Fatal(err)
	}
	if writer.action != "revoke" || writer.key != "evt-91:release_operations:revoke" {
		t.Fatalf("recorded action=%q key=%q", writer.action, writer.key)
	}
	if got.State != "revoked" {
		t.Fatalf("state = %q", got.State)
	}
}
